// Package server запускает HTTP-сервер приложения.
package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"go-uptime/config"
	"go-uptime/database"
	"go-uptime/handlers"
	"go-uptime/internal/applog"
	"go-uptime/middlewares"
	"go-uptime/models"
	"go-uptime/worker"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Run запускает HTTP-сервер и фоновый worker мониторинга.
// Порядок: worker Start → HTTP Listen → по SIGINT/SIGTERM Shutdown HTTP, затем defer Stop worker.
func Run(cfg *config.Config, migrations embed.FS) {
	// migrations передаётся из main для единообразия CLI; HTTP-сервер миграции не применяет.
	_ = migrations

	// Настраиваем zerolog и режим Gin до создания роутера.
	setupLogger(cfg.LogLevel)
	gin.SetMode(cfg.GinMode)

	// gin.New() без дефолтного Logger — логирование через ZerologLogger middleware.
	r := gin.New()
	r.Use(middlewares.ZerologLogger())
	r.Use(middlewares.ErrorCapture()) // ошибки handler'ов попадают в in-memory applog
	r.Use(gin.Recovery())               // panic не роняет процесс

	// Подключение к PostgreSQL, шаблоны и фоновый worker — общие зависимости handler'ов.
	gormDB := database.Connect(cfg.Database)
	tmpl := loadHTMLTemplates(r)
	monitorWorker := worker.New(gormDB, cfg)
	h := handlers.NewHandler(gormDB, tmpl, cfg, monitorWorker)

	r.Static("/static", "./static")

	// Cookie-сессия: username хранится в подписанной cookie, без server-side store.
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30, // 30 суток
		HttpOnly: true,
		Secure:   cfg.SessionSecure, // true за HTTPS в production
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("go_uptime_session", store))

	// Публичные эндпоинты — без AuthRequired.
	r.GET("/health", h.Health)

	r.GET("/", func(c *gin.Context) {
		// Корень сайта ведёт в админку; неавторизованных перехватит AuthRequired на /admin/*.
		c.Redirect(http.StatusFound, "/admin/")
	})

	r.GET("/login", h.LoginPage)
	r.POST("/login", h.Login)

	// Вся админка за middleware AuthRequired — без сессии редирект на /login.
	admin := r.Group("/admin")
	admin.Use(middlewares.AuthRequired())
	{
		admin.GET("/", h.AdminDashboard)
		admin.GET("/users", h.UsersList)
		admin.GET("/users/new", h.NewUserPage)
		admin.POST("/users", h.CreateUser)
		admin.GET("/users/:id/edit", h.EditUserPage)
		admin.POST("/users/:id", h.UpdateUser)
		admin.POST("/users/:id/delete", h.DeleteUser)

		admin.GET("/monitors", h.MonitorsList)
		admin.GET("/monitors/new", h.NewMonitorPage)
		admin.POST("/monitors", h.CreateMonitor)
		admin.GET("/monitors/bulk/new", h.BulkNewMonitorPage)
		admin.POST("/monitors/bulk", h.BulkCreateMonitors)
		admin.GET("/monitors/:id", h.MonitorShowPage)
		admin.GET("/monitors/:id/edit", h.EditMonitorPage)
		admin.POST("/monitors/:id", h.UpdateMonitor)
		admin.POST("/monitors/:id/delete", h.DeleteMonitor)

		admin.GET("/heartbeats", h.HeartbeatsList)

		admin.GET("/incidents", h.IncidentsList)

		admin.GET("/info", h.InfoPage)

		admin.GET("/errors", h.ErrorsPage)
		admin.GET("/logs", h.LogsPage)
		admin.GET("/requests", h.RequestsPage)

		admin.GET("/settings", h.SettingsPage)
		admin.POST("/settings", h.UpdateSettings)

		admin.POST("/logout", h.Logout)

		// Dev tools доступны только в development — middleware проверяет GO_UPTIME_ENVIRONMENT.
		tools := admin.Group("/tools")
		tools.Use(middlewares.DevelopmentOnly(cfg))
		{
			tools.GET("/", h.ToolsPage)
			tools.POST("/clear-table", h.ToolsClearTable)
			tools.POST("/execute-sql", h.ToolsExecuteSQL)
			tools.POST("/seed-monitors", h.ToolsSeedMonitors)
			tools.POST("/test-telegram", h.ToolsTestTelegram)
			tools.POST("/test-smtp", h.ToolsTestSMTP)
			tools.POST("/test-error", h.ToolsTestError)
			tools.POST("/test-log", h.ToolsTestLog)
		}
	}

	// Playwright API — деструктивные REST-эндпоинты для e2e; только development.
	if cfg.EnablePlaywrightAPI {
		if !cfg.IsDevelopment() {
			log.Fatal().Msg("Playwright API requires GO_UPTIME_ENVIRONMENT=development")
		}
		pw := r.Group("/api/playwright")
		{
			pw.POST("/sql", h.PlaywrightExecuteSQL)
			pw.POST("/clear-table", h.PlaywrightClearTable)
			pw.POST("/create-user", h.PlaywrightCreateUser)
			pw.POST("/clear-applog", h.PlaywrightClearApplog)
			pw.POST("/seed-applog", h.PlaywrightSeedApplog)
		}
	}

	// Playwright e2e очищает таблицы и заполняет in-memory логи, пока процесс работает.
	// Worker остаётся «running» для /health, но проверки приостанавливаются, чтобы они не гонялись
	// с TRUNCATE/seed и не портили assertions (лишние heartbeats, monitor-request в applog).
	if cfg.EnablePlaywrightAPI {
		// Pause до Start: e2e не должны гоняться с TRUNCATE/seed параллельно с проверками.
		monitorWorker.Pause()
	}
	// Worker стартует до HTTP — фоновые проверки идут, пока сервер принимает запросы.
	monitorWorker.Start()
	defer monitorWorker.Stop() // Stop выполнится при выходе из Run, после Shutdown HTTP

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		// 65s > urlcheck.RequestTimeout (30s): handler успевает дождаться probe/check до обрыва записи ответа.
		WriteTimeout: 65 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ListenAndServe блокирует goroutine; основной поток ждёт SIGINT/SIGTERM.
	go func() {
		log.Info().Str("port", cfg.HTTPPort).Msg("starting server")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Graceful shutdown: ждём сигнал ОС, затем мягко закрываем HTTP-сервер.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down server")

	// Сначала корректно останавливаем HTTP; defer monitorWorker.Stop() выполнится при выходе из Run.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}
}

func setupLogger(level string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	l, err := zerolog.ParseLevel(level)
	if err != nil {
		// Некорректный GO_UPTIME_LOG_LEVEL — безопасный fallback на info.
		l = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(l)

	// MultiLevelWriter: stderr для оператора + CaptureWriter в память для /admin/logs.
	capture := applog.NewCaptureWriter()
	log.Logger = zerolog.New(
		zerolog.MultiLevelWriter(os.Stderr, capture),
	).Level(l).With().Timestamp().Logger()
}

func loadHTMLTemplates(r *gin.Engine) *template.Template {
	funcMap := template.FuncMap{
		"monitorStatus":          monitorStatusLabel,
		"monitorStatusClass":     monitorStatusClass,
		"monitorDisplayName":     models.MonitorDisplayName,
		"formatUptimePercent":    formatUptimePercent,
		"checkIntervalFormValue": checkIntervalFormValue,
		"uptimePercentClass":     uptimePercentClass,
		"formatResponseTime":     formatResponseTime,
	}

	files := []string{"templates/admin/layout.html"}
	patterns := []string{
		"templates/admin/*.html",
		"templates/admin/*/*.html",
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			log.Fatal().Err(err).Str("pattern", pattern).Msg("failed to glob templates")
		}
		for _, match := range matches {
			if match != "templates/admin/layout.html" {
				files = append(files, match)
			}
		}
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles(files...))
	r.SetHTMLTemplate(tmpl)
	return tmpl
}

func monitorStatusLabel(isUp *bool, lastChecked *time.Time) string {
	if lastChecked == nil {
		// Монитор ещё ни разу не проверялся — статус неизвестен.
		return "Unknown"
	}
	if isUp != nil && *isUp {
		return "Up"
	}
	return "Down"
}

func monitorStatusClass(isUp *bool, lastChecked *time.Time) string {
	if lastChecked == nil {
		return "text-bg-secondary" // серый бейдж для Unknown
	}
	if isUp != nil && *isUp {
		return "text-bg-success"
	}
	return "text-bg-danger"
}

// formatResponseTime форматирует сохранённое время ответа в миллисекундах для шаблонов.
func formatResponseTime(ms *int) string {
	if ms == nil {
		return "—"
	}
	return fmt.Sprintf("%d ms", *ms)
}

// formatUptimePercent форматирует строку процента uptime или прочерк, если данных нет.
// summary — агрегированное окно uptime, показываемое в admin-шаблонах.
func formatUptimePercent(summary models.UptimeSummary) string {
	if !summary.HasData() {
		return "—" // недостаточно проверок для расчёта процента
	}
	return fmt.Sprintf("%.2f%%", summary.Percent())
}

// checkIntervalFormValue возвращает интервал монитора для HTML-форм.
// monitor — редактируемый MonitorURL; пустая строка означает наследование глобальной настройки.
func checkIntervalFormValue(monitor models.MonitorURL) string {
	if monitor.CheckIntervalSeconds == nil {
		return ""
	}
	return strconv.Itoa(*monitor.CheckIntervalSeconds)
}

// uptimePercentClass возвращает Bootstrap CSS-класс цвета текста для значения процента uptime.
func uptimePercentClass(summary models.UptimeSummary) string {
	if !summary.HasData() {
		return "text-muted"
	}
	pct := summary.Percent()
	// Цвет текста по порогам SLA: зелёный ≥99%, жёлтый ≥95%, иначе красный.
	switch {
	case pct >= 99:
		return "text-success"
	case pct >= 95:
		return "text-warning"
	default:
		return "text-danger"
	}
}

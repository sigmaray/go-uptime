// Package server starts the application HTTP server.
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

// Run starts the HTTP server and background monitoring worker.
func Run(cfg *config.Config, migrations embed.FS) {
	_ = migrations

	setupLogger(cfg.LogLevel)
	gin.SetMode(cfg.GinMode)

	r := gin.New()
	r.Use(middlewares.ZerologLogger())
	r.Use(middlewares.ErrorCapture())
	r.Use(gin.Recovery())

	gormDB := database.Connect(cfg.Database)
	tmpl := loadHTMLTemplates(r)
	monitorWorker := worker.New(gormDB, cfg)
	h := handlers.NewHandler(gormDB, tmpl, cfg, monitorWorker)

	r.Static("/static", "./static")

	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   cfg.SessionSecure,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("go_uptime_session", store))

	r.GET("/health", h.Health)

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/admin/")
	})

	r.GET("/login", h.LoginPage)
	r.POST("/login", h.Login)

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

	if cfg.EnablePlaywrightAPI {
		pw := r.Group("/api/playwright")
		{
			pw.POST("/sql", h.PlaywrightExecuteSQL)
			pw.POST("/clear-table", h.PlaywrightClearTable)
			pw.POST("/create-user", h.PlaywrightCreateUser)
			pw.POST("/clear-applog", h.PlaywrightClearApplog)
			pw.POST("/seed-applog", h.PlaywrightSeedApplog)
		}
	}

	// Playwright e2e truncates tables and seeds in-memory logs while the process runs.
	// Keep the worker "running" for /health, but pause checks so they cannot race clears
	// or append extra heartbeats / monitor-request rows mid-assertion.
	if cfg.EnablePlaywrightAPI {
		monitorWorker.Pause()
	}
	monitorWorker.Start()
	defer monitorWorker.Stop()

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 65 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info().Str("port", cfg.HTTPPort).Msg("starting server")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down server")

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
		l = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(l)

	capture := applog.NewCaptureWriter()
	log.Logger = zerolog.New(
		zerolog.MultiLevelWriter(os.Stderr, capture),
	).Level(l).With().Timestamp().Logger().Hook(applogHook{})
}

type applogHook struct{}

func (applogHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level >= zerolog.WarnLevel {
		applog.AddError(msg, level.String())
	}
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
		return "Unknown"
	}
	if isUp != nil && *isUp {
		return "Up"
	}
	return "Down"
}

func monitorStatusClass(isUp *bool, lastChecked *time.Time) string {
	if lastChecked == nil {
		return "text-bg-secondary"
	}
	if isUp != nil && *isUp {
		return "text-bg-success"
	}
	return "text-bg-danger"
}

// formatResponseTime renders a stored response time in milliseconds for templates.
func formatResponseTime(ms *int) string {
	if ms == nil {
		return "—"
	}
	return fmt.Sprintf("%d ms", *ms)
}

// formatUptimePercent renders an uptime percentage string or a dash when data is missing.
// summary is the aggregated uptime window shown in admin templates.
func formatUptimePercent(summary models.UptimeSummary) string {
	if !summary.HasData() {
		return "—"
	}
	return fmt.Sprintf("%.2f%%", summary.Percent())
}

// checkIntervalFormValue returns the monitor interval for HTML forms.
// monitor is the MonitorURL being edited; an empty string means inherit the global setting.
func checkIntervalFormValue(monitor models.MonitorURL) string {
	if monitor.CheckIntervalSeconds == nil {
		return ""
	}
	return strconv.Itoa(*monitor.CheckIntervalSeconds)
}

// uptimePercentClass returns a Bootstrap text color class for an uptime percentage value.
func uptimePercentClass(summary models.UptimeSummary) string {
	if !summary.HasData() {
		return "text-muted"
	}
	pct := summary.Percent()
	switch {
	case pct >= 99:
		return "text-success"
	case pct >= 95:
		return "text-warning"
	default:
		return "text-danger"
	}
}

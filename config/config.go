// Package config загружает типизированную конфигурацию приложения из переменных окружения.
package config

import (
	"fmt"
	"os"

	"github.com/kelseyhightower/envconfig"
)

// Config содержит все настройки приложения, загружаемые при старте.
type Config struct {
	// Environment — режим развёртывания (development, production и т.д.).
	// Влияет на три связанных механизма:
	//   • Dev Tools (/admin/tools) — доступны только при development (middleware DevelopmentOnly).
	//   • Playwright REST API — EnablePlaywrightAPI разрешён только в development; Load отклонит конфиг иначе.
	//   • SessionSecure — вне development Load включает Secure cookies по умолчанию, если GO_UPTIME_SESSION_SECURE не задан явно.
	Environment string `envconfig:"GO_UPTIME_ENVIRONMENT" default:"development"`
	HTTPPort    string `envconfig:"GO_UPTIME_HTTP_PORT" default:"8080"`
	GinMode     string `envconfig:"GIN_MODE" default:"release"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	SessionSecret string `envconfig:"GO_UPTIME_SESSION_SECRET" required:"true"`
	SessionSecure bool   `envconfig:"GO_UPTIME_SESSION_SECURE" default:"false"`

	// IncidentRetentionDays — сколько дней хранить закрытые инциденты после resolved_at.
	// Используется worker/maintenance.go (runMaintenance → models.PruneIncidents) раз в минуту.
	IncidentRetentionDays int `envconfig:"GO_UPTIME_INCIDENT_RETENTION_DAYS" default:"90"`

	// MaxResolvedIncidentsPerMonitor — максимум закрытых инцидентов на один монитор; старые удаляются сверх лимита.
	// Тоже читается в maintenance loop вместе с IncidentRetentionDays.
	MaxResolvedIncidentsPerMonitor int `envconfig:"GO_UPTIME_MAX_RESOLVED_INCIDENTS_PER_MONITOR" default:"100"`

	// CheckConcurrency — сколько HTTP-проверок мониторов worker выполняет параллельно.
	// Задаёт размер «волны» claim: worker захватывает до CheckConcurrency×2 due-мониторов за tick
	// (claimWaveMultiplier), чтобы следующий claim мог стартовать, пока медленные проверки ещё идут.
	CheckConcurrency int `envconfig:"GO_UPTIME_CHECK_CONCURRENCY" default:"150"`

	// EnablePlaywrightAPI открывает деструктивный REST API для e2e (/api/playwright/*).
	// Разрешено только когда Environment=development; иначе Load вернёт ошибку.
	EnablePlaywrightAPI bool `envconfig:"GO_UPTIME_ENABLE_PLAYWRIGHT_API" default:"false"`

	Database DatabaseConfig
}

// DatabaseConfig содержит параметры подключения к PostgreSQL.
type DatabaseConfig struct {
	Host     string `envconfig:"GO_UPTIME_DATABASE_HOST" default:"localhost"`
	Port     string `envconfig:"GO_UPTIME_DATABASE_PORT" default:"5432"`
	User     string `envconfig:"GO_UPTIME_DATABASE_USER" default:"gouptime"`
	DBName   string `envconfig:"GO_UPTIME_DATABASE_NAME" default:"gouptime"`
	Password string `envconfig:"GO_UPTIME_DATABASE_PASSWORD" required:"true"`
	// SSLMode — значение libpq sslmode (disable, require, verify-full и т.д.).
	SSLMode string `envconfig:"GO_UPTIME_DATABASE_SSLMODE" default:"disable"`
}

// IsDevelopment возвращает true, когда приложение работает в режиме development.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// Load читает конфигурацию из переменных окружения.
// Вне development по умолчанию устанавливает SessionSecure в true, если GO_UPTIME_SESSION_SECURE не задан,
// отклоняет Playwright API вне development и нормализует пустой SSLMode в disable.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}

	// Вне development Secure cookies включаются по умолчанию, если явно не настроено иначе.
	if !cfg.IsDevelopment() && os.Getenv("GO_UPTIME_SESSION_SECURE") == "" {
		cfg.SessionSecure = true
	}

	if cfg.EnablePlaywrightAPI && !cfg.IsDevelopment() {
		// Playwright API деструктивен — только в development.
		return nil, fmt.Errorf("GO_UPTIME_ENABLE_PLAYWRIGHT_API requires GO_UPTIME_ENVIRONMENT=development")
	}

	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}

	return &cfg, nil
}

// Package config загружает типизированную конфигурацию приложения из переменных окружения.
package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config содержит все настройки приложения, загружаемые при старте.
type Config struct {
	Environment string `envconfig:"GO_UPTIME_ENVIRONMENT" default:"development"`
	HTTPPort    string `envconfig:"GO_UPTIME_HTTP_PORT" default:"8080"`
	GinMode     string `envconfig:"GIN_MODE" default:"release"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	SessionSecret string `envconfig:"GO_UPTIME_SESSION_SECRET" required:"true"`
	SessionSecure bool   `envconfig:"GO_UPTIME_SESSION_SECURE" default:"false"`

	// CheckIntervalSeconds — интервал фоновых проверок URL по умолчанию (переопределяется в БД).
	CheckIntervalSeconds int `envconfig:"GO_UPTIME_CHECK_INTERVAL_SECONDS" default:"60"`

	// IncidentRetentionDays — сколько дней хранить закрытые инциденты.
	IncidentRetentionDays int `envconfig:"GO_UPTIME_INCIDENT_RETENTION_DAYS" default:"90"`

	// MaxResolvedIncidentsPerMonitor — лимит закрытых инцидентов на один URL.
	MaxResolvedIncidentsPerMonitor int `envconfig:"GO_UPTIME_MAX_RESOLVED_INCIDENTS_PER_MONITOR" default:"100"`

	EnablePlaywrightAPI bool `envconfig:"GO_UPTIME_ENABLE_PLAYWRIGHT_API" default:"false"`

	Database DatabaseConfig
}

// DatabaseConfig — параметры подключения к PostgreSQL.
type DatabaseConfig struct {
	Host     string `envconfig:"GO_UPTIME_DATABASE_HOST" default:"localhost"`
	Port     string `envconfig:"GO_UPTIME_DATABASE_PORT" default:"5432"`
	User     string `envconfig:"GO_UPTIME_DATABASE_USER" default:"gouptime"`
	DBName   string `envconfig:"GO_UPTIME_DATABASE_NAME" default:"gouptime"`
	Password string `envconfig:"GO_UPTIME_DATABASE_PASSWORD" required:"true"`
}

// IsDevelopment возвращает true, если приложение запущено в режиме разработки.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// Load читает конфигурацию из переменных окружения.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return &cfg, nil
}

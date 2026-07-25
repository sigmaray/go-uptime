// Package config loads typed application configuration from environment variables.
package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application settings loaded at startup.
type Config struct {
	Environment string `envconfig:"GO_UPTIME_ENVIRONMENT" default:"development"`
	HTTPPort    string `envconfig:"GO_UPTIME_HTTP_PORT" default:"8080"`
	GinMode     string `envconfig:"GIN_MODE" default:"release"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	SessionSecret string `envconfig:"GO_UPTIME_SESSION_SECRET" required:"true"`
	SessionSecure bool   `envconfig:"GO_UPTIME_SESSION_SECURE" default:"false"`

	// IncidentRetentionDays is how many days to keep resolved incidents.
	IncidentRetentionDays int `envconfig:"GO_UPTIME_INCIDENT_RETENTION_DAYS" default:"90"`

	// MaxResolvedIncidentsPerMonitor is the limit of resolved incidents per URL.
	MaxResolvedIncidentsPerMonitor int `envconfig:"GO_UPTIME_MAX_RESOLVED_INCIDENTS_PER_MONITOR" default:"100"`

	// CheckConcurrency is how many HTTP monitor checks may run at once.
	CheckConcurrency int `envconfig:"GO_UPTIME_CHECK_CONCURRENCY" default:"150"`

	EnablePlaywrightAPI bool `envconfig:"GO_UPTIME_ENABLE_PLAYWRIGHT_API" default:"false"`

	Database DatabaseConfig
}

// DatabaseConfig holds PostgreSQL connection parameters.
type DatabaseConfig struct {
	Host     string `envconfig:"GO_UPTIME_DATABASE_HOST" default:"localhost"`
	Port     string `envconfig:"GO_UPTIME_DATABASE_PORT" default:"5432"`
	User     string `envconfig:"GO_UPTIME_DATABASE_USER" default:"gouptime"`
	DBName   string `envconfig:"GO_UPTIME_DATABASE_NAME" default:"gouptime"`
	Password string `envconfig:"GO_UPTIME_DATABASE_PASSWORD" required:"true"`
}

// IsDevelopment returns true when the application runs in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return &cfg, nil
}

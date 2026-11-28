package config

import (
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestDatabaseConnectionURLEncodesCredentials checks that special characters stay inside URL values.
func TestDatabaseConnectionURLEncodesCredentials(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "db.example.test",
		Port:     "5432",
		User:     "user name",
		Password: "p@ss word:/?#",
		DBName:   "uptime test",
		SSLMode:  "verify-full",
	}

	parsed, err := url.Parse(cfg.ConnectionURL())
	if err != nil {
		t.Fatalf("parse connection URL: %v", err)
	}
	password, ok := parsed.User.Password()
	if !ok {
		t.Fatal("connection URL does not contain a password")
	}
	if parsed.User.Username() != cfg.User || password != cfg.Password {
		t.Fatalf("decoded credentials = (%q, %q), want (%q, %q)", parsed.User.Username(), password, cfg.User, cfg.Password)
	}
	if got := parsed.Path; got != "/"+cfg.DBName {
		t.Fatalf("database path = %q, want %q", got, "/"+cfg.DBName)
	}
	if got := parsed.Query().Get("sslmode"); got != cfg.SSLMode {
		t.Fatalf("sslmode = %q, want %q", got, cfg.SSLMode)
	}
	driverConfig, err := pgx.ParseConfig(cfg.ConnectionURL())
	if err != nil {
		t.Fatalf("parse PostgreSQL connection URL: %v", err)
	}
	if driverConfig.User != cfg.User || driverConfig.Password != cfg.Password || driverConfig.Database != cfg.DBName {
		t.Fatalf("driver decoded user/password/database incorrectly: user=%q database=%q", driverConfig.User, driverConfig.Database)
	}
}

// TestDatabaseConnectionURLDefaultsSSLMode checks the default mode for an empty SSL setting.
func TestDatabaseConnectionURLDefaultsSSLMode(t *testing.T) {
	cfg := DatabaseConfig{Host: "localhost", Port: "5432", User: "user", Password: "secret", DBName: "uptime"}
	parsed, err := url.Parse(cfg.ConnectionURL())
	if err != nil {
		t.Fatalf("parse connection URL: %v", err)
	}
	if got := parsed.Query().Get("sslmode"); got != "disable" {
		t.Fatalf("sslmode = %q, want disable", got)
	}
}

// TestLoadRejectsPlaywrightOutsideDevelopment ensures the Playwright test API cannot enable in production.
func TestLoadRejectsPlaywrightOutsideDevelopment(t *testing.T) {
	t.Setenv("GO_UPTIME_SESSION_SECRET", "test-secret")
	t.Setenv("GO_UPTIME_DATABASE_PASSWORD", "test-password")
	t.Setenv("GO_UPTIME_ENVIRONMENT", "production")
	t.Setenv("GO_UPTIME_ENABLE_PLAYWRIGHT_API", "true")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Playwright API enabled outside development")
	}
}

// TestLoadDefaultsSessionSecureOutsideDevelopment ensures Secure cookies default on outside development.
func TestLoadDefaultsSessionSecureOutsideDevelopment(t *testing.T) {
	t.Setenv("GO_UPTIME_SESSION_SECRET", "test-secret")
	t.Setenv("GO_UPTIME_DATABASE_PASSWORD", "test-password")
	t.Setenv("GO_UPTIME_ENVIRONMENT", "production")
	_ = os.Unsetenv("GO_UPTIME_SESSION_SECURE")
	t.Setenv("GO_UPTIME_ENABLE_PLAYWRIGHT_API", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SessionSecure {
		t.Fatal("expected SessionSecure=true outside development when unset")
	}
}

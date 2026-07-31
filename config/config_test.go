package config

import (
	"os"
	"testing"
)

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

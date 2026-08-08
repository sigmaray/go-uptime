package config

import (
	"os"
	"testing"
)

// TestLoadRejectsPlaywrightOutsideDevelopment проверяет, что Playwright test API нельзя включить в production.
func TestLoadRejectsPlaywrightOutsideDevelopment(t *testing.T) {
	// Arrange: production env с явно включённым Playwright API.
	t.Setenv("GO_UPTIME_SESSION_SECRET", "test-secret")
	t.Setenv("GO_UPTIME_DATABASE_PASSWORD", "test-password")
	t.Setenv("GO_UPTIME_ENVIRONMENT", "production")
	t.Setenv("GO_UPTIME_ENABLE_PLAYWRIGHT_API", "true")

	// Act: Load должен отклонить небезопасную комбинацию настроек.
	_, err := Load()
	// Assert: ошибка конфигурации обязательна.
	if err == nil {
		t.Fatal("expected error when Playwright API enabled outside development")
	}
}

// TestLoadDefaultsSessionSecureOutsideDevelopment проверяет, что Secure cookies по умолчанию включены вне development.
func TestLoadDefaultsSessionSecureOutsideDevelopment(t *testing.T) {
	// Arrange: production без явного GO_UPTIME_SESSION_SECURE.
	t.Setenv("GO_UPTIME_SESSION_SECRET", "test-secret")
	t.Setenv("GO_UPTIME_DATABASE_PASSWORD", "test-password")
	t.Setenv("GO_UPTIME_ENVIRONMENT", "production")
	_ = os.Unsetenv("GO_UPTIME_SESSION_SECURE")
	t.Setenv("GO_UPTIME_ENABLE_PLAYWRIGHT_API", "false")

	// Act: загружаем конфиг с дефолтами для production.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Assert: SessionSecure=true по умолчанию вне development.
	if !cfg.SessionSecure {
		t.Fatal("expected SessionSecure=true outside development when unset")
	}
}

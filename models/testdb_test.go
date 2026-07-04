package models

import (
	"fmt"
	"os"
	"testing"

	"go-uptime/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := os.Getenv("GO_UPTIME_TEST_DATABASE_NAME")
	if dbName == "" {
		t.Skip("GO_UPTIME_TEST_DATABASE_NAME is not set")
	}

	cfg := config.DatabaseConfig{
		Host:     envOrDefault("GO_UPTIME_DATABASE_HOST", "localhost"),
		Port:     envOrDefault("GO_UPTIME_DATABASE_PORT", "5432"),
		User:     envOrDefault("GO_UPTIME_DATABASE_USER", "postgres"),
		Password: envOrDefault("GO_UPTIME_DATABASE_PASSWORD", "postgres"),
		DBName:   dbName,
	}

	db, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return db
}

func resetUptimeStatTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"stat_minutely", "stat_hourly", "stat_daily", "monitor_checks", "monitor_urls"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

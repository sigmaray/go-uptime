package models

import (
	"fmt"
	"os"
	"testing"

	"go-uptime/config"
	"go-uptime/internal/testdb"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openTestDB connects to the isolated test database and ensures its schema exists.
// t is the active test used for fatal error reporting.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := os.Getenv("GO_UPTIME_TEST_DATABASE_NAME")
	if dbName == "" {
		t.Skip("GO_UPTIME_TEST_DATABASE_NAME is not set")
	}
	if err := testdb.ValidateTestDatabaseName(dbName, envOrDefault("GO_UPTIME_DATABASE_NAME", "gouptime")); err != nil {
		t.Fatal(err)
	}

	cfg := config.DatabaseConfig{
		Host:     envOrDefault("GO_UPTIME_DATABASE_HOST", "localhost"),
		Port:     envOrDefault("GO_UPTIME_DATABASE_PORT", "5432"),
		User:     envOrDefault("GO_UPTIME_DATABASE_USER", "postgres"),
		Password: envOrDefault("GO_UPTIME_DATABASE_PASSWORD", "postgres"),
		DBName:   dbName,
	}

	ensureTestDatabase(t, cfg)

	db, err := gorm.Open(postgres.Open(cfg.ConnectionURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	testdb.ApplyMigrations(t, db)

	return db
}

// ensureTestDatabase creates the test database when PostgreSQL does not have it yet.
// t is the active test used for fatal error reporting.
// cfg holds connection settings with DBName set to the test database name.
func ensureTestDatabase(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()

	adminConfig := cfg
	adminConfig.DBName = "postgres"
	adminDB, err := gorm.Open(postgres.Open(adminConfig.ConnectionURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres admin db: %v", err)
	}

	sqlDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("postgres admin handle: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var exists int64
	if err := adminDB.Raw("SELECT COUNT(1) FROM pg_database WHERE datname = ?", cfg.DBName).Scan(&exists).Error; err != nil {
		t.Fatalf("lookup test database: %v", err)
	}
	if exists > 0 {
		return
	}

	if err := adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.DBName)).Error; err != nil {
		t.Fatalf("create test database: %v", err)
	}
}

// resetUptimeStatTables truncates uptime-related tables for isolated test cases.
// t is the active test used for fatal error reporting.
// db is the test database connection whose tables are truncated.
func resetUptimeStatTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"stat_minutely", "stat_hourly", "stat_daily", "monitor_checks", "monitor_urls"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// envOrDefault returns the environment variable value or the provided fallback.
// key is the environment variable name.
// fallback is used when the variable is unset or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

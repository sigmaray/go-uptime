// Package testdb provides helpers for integration tests that use the project migrations.
package testdb

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// ValidateTestDatabaseName rejects empty, application, and PostgreSQL system database names.
// testName is the database name reserved for tests; applicationName is the configured application database.
func ValidateTestDatabaseName(testName, applicationName string) error {
	if strings.TrimSpace(testName) == "" {
		return fmt.Errorf("GO_UPTIME_TEST_DATABASE_NAME must not be empty")
	}
	if strings.EqualFold(testName, applicationName) {
		return fmt.Errorf("test database %q must be different from the application database", testName)
	}
	switch strings.ToLower(testName) {
	case "postgres", "template0", "template1":
		return fmt.Errorf("test database %q is a PostgreSQL system database", testName)
	}
	return nil
}

// ApplyMigrations applies the application's Goose migrations to the isolated test database.
// It resets a legacy AutoMigrate schema so tests exercise the same schema as the application.
// t is the active test used for failure reporting; db is the GORM connection to the test database.
func ApplyMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()

	var hasMigrationHistory bool
	if err := db.Raw("SELECT to_regclass('public.goose_db_version') IS NOT NULL").Scan(&hasMigrationHistory).Error; err != nil {
		t.Fatalf("check test database migration history: %v", err)
	}
	if !hasMigrationHistory {
		var tableCount int64
		if err := db.Raw("SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public'").Scan(&tableCount).Error; err != nil {
			t.Fatalf("inspect test database schema: %v", err)
		}
		if tableCount > 0 {
			if err := db.Exec("DROP SCHEMA public CASCADE").Error; err != nil {
				t.Fatalf("reset legacy test database schema: %v", err)
			}
			if err := db.Exec("CREATE SCHEMA public").Error; err != nil {
				t.Fatalf("recreate test database schema: %v", err)
			}
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get test database handle: %v", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set Goose dialect: %v", err)
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test migration helper")
	}
	migrationDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "migrations")
	if err := goose.Up(sqlDB, migrationDir); err != nil {
		t.Fatalf("apply Goose migrations to test database: %v", err)
	}
}

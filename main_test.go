package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func runCLI(t *testing.T, envs []string, input string, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Env = append(os.Environ(), envs...)

	if input != "" {
		ptmx, err := pty.Start(cmd)
		if err != nil {
			t.Fatalf("pty.Start failed: %v", err)
		}

		go func() {
			_, _ = ptmx.WriteString(input)
		}()

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, ptmx)
		_ = cmd.Wait()
		_ = ptmx.Close()
		return buf.String(), buf.String()
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return stdout.String(), stderr.String()
}

func getTestDBName() string {
	return os.Getenv("GO_UPTIME_TEST_DATABASE_NAME")
}

func testEnvs() []string {
	return []string{
		fmt.Sprintf("GO_UPTIME_DATABASE_NAME=%s", getTestDBName()),
		fmt.Sprintf("GO_UPTIME_DATABASE_HOST=%s", envOr("GO_UPTIME_DATABASE_HOST", "localhost")),
		fmt.Sprintf("GO_UPTIME_DATABASE_PORT=%s", envOr("GO_UPTIME_DATABASE_PORT", "5432")),
		fmt.Sprintf("GO_UPTIME_DATABASE_USER=%s", envOr("GO_UPTIME_DATABASE_USER", "postgres")),
		fmt.Sprintf("GO_UPTIME_DATABASE_PASSWORD=%s", envOr("GO_UPTIME_DATABASE_PASSWORD", "postgres")),
		"GO_UPTIME_SESSION_SECRET=testsecret",
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func setupTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		envOr("GO_UPTIME_DATABASE_HOST", "localhost"),
		envOr("GO_UPTIME_DATABASE_PORT", "5432"),
		envOr("GO_UPTIME_DATABASE_USER", "postgres"),
		envOr("GO_UPTIME_DATABASE_PASSWORD", "postgres"),
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer func() { _ = db.Close() }()

	testDB := getTestDBName()
	if _, err := db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, testDB)); err != nil {
		t.Fatalf("drop test db: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, testDB)); err != nil {
		t.Fatalf("create test db: %v", err)
	}
}

func cleanupTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		envOr("GO_UPTIME_DATABASE_HOST", "localhost"),
		envOr("GO_UPTIME_DATABASE_PORT", "5432"),
		envOr("GO_UPTIME_DATABASE_USER", "postgres"),
		envOr("GO_UPTIME_DATABASE_PASSWORD", "postgres"),
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Errorf("connect postgres: %v", err)
		return
	}
	defer func() { _ = db.Close() }()

	testDB := getTestDBName()
	_, _ = db.Exec(fmt.Sprintf(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = '%s' AND pid <> pg_backend_pid()
	`, testDB))
	if _, err := db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, testDB)); err != nil {
		t.Errorf("drop test db: %v", err)
	}
}

func TestCLICommands(t *testing.T) {
	_ = godotenv.Load()
	if getTestDBName() == "" {
		t.Skip("GO_UPTIME_TEST_DATABASE_NAME is not set")
	}

	setupTestDB(t)
	defer cleanupTestDB(t)

	envs := testEnvs()

	t.Run("Help", func(t *testing.T) {
		stdout, _ := runCLI(t, envs, "")
		if !strings.Contains(stdout, "db-users-create") {
			t.Fatalf("expected help with commands, got: %s", stdout)
		}
	})

	t.Run("GooseMigrate", func(t *testing.T) {
		_, stderr := runCLI(t, envs, "", "db-goose-migrate")
		if strings.Contains(stderr, "fatal") {
			t.Fatalf("migration failed: %s", stderr)
		}
	})

	t.Run("UsersSeed", func(t *testing.T) {
		stdout, _ := runCLI(t, envs, "y\n", "db-users-seed")
		if !strings.Contains(stdout, "username=admin") {
			t.Fatalf("expected admin user, got: %s", stdout)
		}
	})

	t.Run("UsersShow", func(t *testing.T) {
		stdout, _ := runCLI(t, envs, "", "db-users-show")
		if !strings.Contains(stdout, "admin") {
			t.Fatalf("expected admin in table, got: %s", stdout)
		}
	})

	t.Run("UsersCreate", func(t *testing.T) {
		input := "testcli\npass123\npass123\n"
		stdout, _ := runCLI(t, envs, input, "db-users-create")
		if !strings.Contains(stdout, "username=testcli") {
			t.Fatalf("expected testcli created, got: %s", stdout)
		}
	})

	t.Run("ExecuteSQL", func(t *testing.T) {
		stdout, _ := runCLI(t, envs, "", "db-execute-sql", "SELECT username FROM users ORDER BY id")
		if !strings.Contains(stdout, "admin") || !strings.Contains(stdout, "testcli") {
			t.Fatalf("expected sql table output, got: %s", stdout)
		}
	})

	t.Run("ClearTable", func(t *testing.T) {
		stdout, _ := runCLI(t, envs, "y\n", "db-clear-table", "users")
		if !strings.Contains(stdout, "cleared") {
			t.Fatalf("expected table cleared, got: %s", stdout)
		}
		stdout, _ = runCLI(t, envs, "", "db-users-show")
		if !strings.Contains(stdout, "No users found") {
			t.Fatalf("expected no users, got: %s", stdout)
		}
	})

	t.Run("GormMigrate", func(t *testing.T) {
		_, stderr := runCLI(t, envs, "", "db-gorm-migrate")
		if strings.Contains(stderr, "fatal") {
			t.Fatalf("gorm migrate failed: %s", stderr)
		}
	})
}

package main

import (
	"bytes"
	"context"
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

// runCLI запускает приложение через `go run .` с заданными env, stdin и аргументами CLI.
// t — активный тест; envs дополняют окружение процесса; input пишется в pseudo-TTY при непустом значении.
// Возвращает stdout и stderr (при input оба указывают на один буфер PTY).
func runCLI(t *testing.T, envs []string, input string, args ...string) (string, string) {
	t.Helper()
	cmdArgs := append([]string{"run", "."}, args...)
	// Тестовый helper всегда вызывает локальный модуль через фиксированный «go run .» плюс аргументы теста.
	cmd := exec.CommandContext(context.Background(), "go", cmdArgs...) //nolint:gosec // G204: фиксированный вызов go toolchain в тестах
	cmd.Env = append(os.Environ(), envs...)

	if input != "" {
		// Интерактивные команды (seed, clear-table) — через PTY для эмуляции ввода пользователя.
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

	// Неинтерактивный режим — раздельный захват stdout/stderr.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return stdout.String(), stderr.String()
}

// getTestDBName возвращает имя базовой тестовой БД из GO_UPTIME_TEST_DATABASE_NAME.
func getTestDBName() string {
	return os.Getenv("GO_UPTIME_TEST_DATABASE_NAME")
}

// testEnvs формирует env для CLI-подпроцесса: отдельная БД *_cli и параметры подключения к PostgreSQL.
func testEnvs() []string {
	return []string{
		fmt.Sprintf("GO_UPTIME_DATABASE_NAME=%s_cli", getTestDBName()),
		fmt.Sprintf("GO_UPTIME_DATABASE_HOST=%s", envOr("GO_UPTIME_DATABASE_HOST", "localhost")),
		fmt.Sprintf("GO_UPTIME_DATABASE_PORT=%s", envOr("GO_UPTIME_DATABASE_PORT", "5432")),
		fmt.Sprintf("GO_UPTIME_DATABASE_USER=%s", envOr("GO_UPTIME_DATABASE_USER", "postgres")),
		fmt.Sprintf("GO_UPTIME_DATABASE_PASSWORD=%s", envOr("GO_UPTIME_DATABASE_PASSWORD", "postgres")),
		"GO_UPTIME_SESSION_SECRET=testsecret",
	}
}

// envOr возвращает значение переменной окружения или fallback, если переменная пуста.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// setupTestDB пересоздаёт изолированную CLI-тестовую БД {GO_UPTIME_TEST_DATABASE_NAME}_cli.
func setupTestDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
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

	testDB := getTestDBName() + "_cli"
	// DROP + CREATE даёт чистое состояние перед каждым прогоном TestCLICommands.
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, testDB)); err != nil {
		t.Fatalf("drop test db: %v", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, testDB)); err != nil {
		t.Fatalf("create test db: %v", err)
	}
}

// cleanupTestDB удаляет CLI-тестовую БД и завершает активные подключения к ней.
func cleanupTestDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
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

	testDB := getTestDBName() + "_cli"
	// pg_terminate_backend нужен, иначе DROP DATABASE может зависнуть на открытых сессиях CLI.
	_, _ = db.ExecContext(ctx, fmt.Sprintf(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = '%s' AND pid <> pg_backend_pid()
	`, testDB))
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, testDB)); err != nil {
		t.Errorf("drop test db: %v", err)
	}
}

func TestCLICommands(t *testing.T) {
	// Arrange: .env и проверка, что задана базовая тестовая БД.
	_ = godotenv.Load()
	if getTestDBName() == "" {
		t.Skip("GO_UPTIME_TEST_DATABASE_NAME is not set")
	}

	setupTestDB(t)
	defer cleanupTestDB(t)

	envs := testEnvs()

	t.Run("Help", func(t *testing.T) {
		// Act: вывод справки без аргументов.
		stdout, _ := runCLI(t, envs, "")
		// Assert: в help есть имя хотя бы одной db-* команды.
		if !strings.Contains(stdout, "db-users-create") {
			t.Fatalf("expected help with commands, got: %s", stdout)
		}
	})

	t.Run("GooseMigrate", func(t *testing.T) {
		// Act: goose migrate на свежей CLI БД.
		_, stderr := runCLI(t, envs, "", "db-goose-migrate")
		// Assert: в stderr нет fatal-ошибки.
		if strings.Contains(stderr, "fatal") {
			t.Fatalf("migration failed: %s", stderr)
		}
	})

	t.Run("UsersSeed", func(t *testing.T) {
		// Act: seed с подтверждением «y» через PTY.
		stdout, _ := runCLI(t, envs, "y\n", "db-users-seed")
		// Assert: создан пользователь admin.
		if !strings.Contains(stdout, "username=admin") {
			t.Fatalf("expected admin user, got: %s", stdout)
		}
	})

	t.Run("UsersShow", func(t *testing.T) {
		// Act: таблица пользователей после seed.
		stdout, _ := runCLI(t, envs, "", "db-users-show")
		// Assert: admin виден в выводе.
		if !strings.Contains(stdout, "admin") {
			t.Fatalf("expected admin in table, got: %s", stdout)
		}
	})

	t.Run("UsersCreate", func(t *testing.T) {
		// Arrange: интерактивный ввод username и пароля дважды.
		input := "testcli\npassword123\npassword123\n"
		// Act: создание нового пользователя через CLI.
		stdout, _ := runCLI(t, envs, input, "db-users-create")
		// Assert: в выводе подтверждение создания testcli.
		if !strings.Contains(stdout, "username=testcli") {
			t.Fatalf("expected testcli created, got: %s", stdout)
		}
	})

	t.Run("ExecuteSQL", func(t *testing.T) {
		// Act: ad-hoc SQL SELECT по users.
		stdout, _ := runCLI(t, envs, "", "db-execute-sql", "SELECT username FROM users ORDER BY id")
		// Assert: оба пользователя (admin и testcli) в табличном выводе.
		if !strings.Contains(stdout, "admin") || !strings.Contains(stdout, "testcli") {
			t.Fatalf("expected sql table output, got: %s", stdout)
		}
	})

	t.Run("ClearTable", func(t *testing.T) {
		// Act: очистка users с подтверждением.
		stdout, _ := runCLI(t, envs, "y\n", "db-clear-table", "users")
		if !strings.Contains(stdout, "cleared") {
			t.Fatalf("expected table cleared, got: %s", stdout)
		}
		// Assert: после очистки список пользователей пуст.
		stdout, _ = runCLI(t, envs, "", "db-users-show")
		if !strings.Contains(stdout, "No users found") {
			t.Fatalf("expected no users, got: %s", stdout)
		}
	})

	t.Run("GormMigrate", func(t *testing.T) {
		// Act: gorm AutoMigrate через CLI.
		_, stderr := runCLI(t, envs, "", "db-gorm-migrate")
		// Assert: миграция завершилась без fatal.
		if strings.Contains(stderr, "fatal") {
			t.Fatalf("gorm migrate failed: %s", stderr)
		}
	})
}

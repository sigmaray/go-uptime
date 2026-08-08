package models

import (
	"fmt"
	"os"
	"testing"

	"go-uptime/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openTestDB подключается к изолированной тестовой базе данных и обеспечивает наличие схемы.
// t — активный тест для сообщений о фатальных ошибках.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Без GO_UPTIME_TEST_DATABASE_NAME интеграционные тесты models пропускаются.
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

	// Создаём БД при первом запуске, если её ещё нет.
	ensureTestDatabase(t, cfg)

	db, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Применяем AutoMigrate для всех таблиц, используемых тестами models.
	if err := migrateTestSchema(db); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}

	return db
}

// ensureTestDatabase создаёт тестовую базу данных, если PostgreSQL её ещё не имеет.
// t — активный тест для сообщений о фатальных ошибках.
// cfg содержит параметры подключения с DBName, равным имени тестовой базы данных.
func ensureTestDatabase(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()

	// Подключение к служебной БД postgres — CREATE DATABASE выполняется оттуда.
	adminDB, err := gorm.Open(postgres.Open(fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password,
	)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres admin db: %v", err)
	}

	sqlDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("postgres admin handle: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// Проверяем наличие целевой БД через pg_catalog.
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

// migrateTestSchema создаёт таблицы, необходимые для тестов моделей.
// db — подключение к тестовой базе данных, которое получает вызовы AutoMigrate.
func migrateTestSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&MonitorURL{},
		&MonitorCheck{},
		&Incident{},
		&AppSetting{},
		&StatMinutely{},
		&StatHourly{},
		&StatDaily{},
	)
}

// resetUptimeStatTables очищает таблицы, связанные с uptime, для изолированных тестовых случаев.
// t — активный тест для сообщений о фатальных ошибках.
// db — подключение к тестовой базе данных, чьи таблицы усекаются.
func resetUptimeStatTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	// TRUNCATE ... RESTART IDENTITY CASCADE сбрасывает PK и FK-зависимости между таблицами.
	for _, table := range []string{"stat_minutely", "stat_hourly", "stat_daily", "monitor_checks", "monitor_urls"} {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// envOrDefault возвращает значение переменной окружения или указанное значение по умолчанию.
// key — имя переменной окружения.
// fallback используется, когда переменная не задана или пуста.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

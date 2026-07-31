// Package database handles PostgreSQL connections and Goose migrations.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"go-uptime/config"
	"go-uptime/models"

	"github.com/pressly/goose/v3"
	gooseLock "github.com/pressly/goose/v3/lock"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DSN builds a PostgreSQL connection string from configuration.
// c holds host, port, credentials, database name, and SSL mode (defaults to disable when empty).
func DSN(c config.DatabaseConfig) string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, sslMode,
	)
}

// Connect opens a PostgreSQL connection through GORM.
func Connect(cfg config.DatabaseConfig) *gorm.DB {
	db, err := gorm.Open(postgres.Open(DSN(cfg)), &gorm.Config{})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	return db
}

// RunGooseMigrations applies embedded Goose SQL migrations.
func RunGooseMigrations(migrations embed.FS, cfg config.DatabaseConfig) {
	db := Connect(cfg)

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get database handle")
	}
	defer closeSQLDB(sqlDB)

	runMigrations(migrations, sqlDB)
}

func runMigrations(migrations embed.FS, sqlDB *sql.DB) {
	migrationFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open migrations directory")
	}

	sessionLocker, err := gooseLock.NewPostgresSessionLocker()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create migration session locker")
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		migrationFS,
		goose.WithSessionLocker(sessionLocker),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create migration provider")
	}

	if _, err := provider.Up(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}
}

// RunGormAutoMigrate creates or updates tables through GORM AutoMigrate.
func RunGormAutoMigrate(cfg config.DatabaseConfig) {
	db := Connect(cfg)
	if err := db.AutoMigrate(
		&models.User{},
		&models.MonitorURL{},
		&models.MonitorCheck{},
		&models.Incident{},
		&models.AppSetting{},
		&models.StatMinutely{},
		&models.StatHourly{},
		&models.StatDaily{},
	); err != nil {
		log.Fatal().Err(err).Msg("failed to auto-migrate")
	}
}

// ListTables returns user table names in the public schema.
func ListTables(db *gorm.DB) ([]string, error) {
	var tables []string
	err := db.Raw(`
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		ORDER BY tablename
	`).Scan(&tables).Error
	return tables, err
}

// ClearTable clears the specified table.
func ClearTable(db *gorm.DB, table string) error {
	table = sanitizeIdentifier(table)
	return db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error
}

// DropTable drops the specified table.
func DropTable(db *gorm.DB, table string) error {
	table = sanitizeIdentifier(table)
	return db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table)).Error
}

// ClearAllTables clears all user tables.
func ClearAllTables(db *gorm.DB) error {
	tables, err := ListTables(db)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if err := ClearTable(db, table); err != nil {
			return err
		}
	}
	return nil
}

// DropAllTables drops all user tables.
func DropAllTables(db *gorm.DB) error {
	tables, err := ListTables(db)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if err := DropTable(db, table); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteSQL runs arbitrary SQL and returns result rows for SELECT queries.
func ExecuteSQL(db *gorm.DB, query string) (columns []string, rows [][]string, rowsAffected int64, err error) {
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	if strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "WITH") {
		sqlRows, qerr := db.Raw(query).Rows()
		if qerr != nil {
			return nil, nil, 0, qerr
		}
		defer func() { _ = sqlRows.Close() }()

		columns, err = sqlRows.Columns()
		if err != nil {
			return nil, nil, 0, err
		}

		for sqlRows.Next() {
			values := make([]interface{}, len(columns))
			ptrs := make([]interface{}, len(columns))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := sqlRows.Scan(ptrs...); err != nil {
				return nil, nil, 0, err
			}
			row := make([]string, len(columns))
			for i, v := range values {
				if v == nil {
					row[i] = "NULL"
				} else {
					row[i] = fmt.Sprint(v)
				}
			}
			rows = append(rows, row)
		}
		return columns, rows, int64(len(rows)), sqlRows.Err()
	}

	result := db.Exec(query)
	return nil, nil, result.RowsAffected, result.Error
}

func sanitizeIdentifier(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("empty table name")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		panic("invalid table name: " + name)
	}
	return name
}

func closeSQLDB(sqlDB *sql.DB) {
	if err := sqlDB.Close(); err != nil {
		log.Error().Err(err).Msg("failed to close database")
	}
}

package models

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// postgresUniqueViolation is the PostgreSQL SQLSTATE for unique_violation.
const postgresUniqueViolation = "23505"

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint violation.
// err is any persistence error returned by GORM or the database driver.
// It is used by handlers to map duplicate-key failures to user-facing conflict responses.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == postgresUniqueViolation
	}
	return false
}

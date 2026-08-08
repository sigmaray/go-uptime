package models

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// postgresUniqueViolation — SQLSTATE PostgreSQL для unique_violation.
const postgresUniqueViolation = "23505"

// IsUniqueViolation сообщает, является ли err нарушением уникального ограничения PostgreSQL.
// err — любая ошибка персистентности, возвращённая GORM или драйвером базы данных.
// Используется обработчиками для преобразования ошибок дублирования ключа в ответы о конфликте для пользователя.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE 23505 — unique_violation в PostgreSQL.
		return pgErr.Code == postgresUniqueViolation
	}
	return false
}

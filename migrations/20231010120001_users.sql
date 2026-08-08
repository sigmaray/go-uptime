-- +goose Up
-- Таблица пользователей: учётные записи для входа в веб-интерфейс и CLI.
-- Пароли хранятся только в виде хеша; мягкое удаление через deleted_at.
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- NULL = активный пользователь; NOT NULL = логически удалён (soft delete).
    deleted_at TIMESTAMPTZ,
    -- Логин для аутентификации; уникальность обеспечивается частичным индексом ниже.
    username TEXT NOT NULL,
    -- Хеш пароля (не plaintext); используется при проверке credentials.
    password_hash TEXT NOT NULL
);

-- Уникальный username только среди неудалённых записей: можно повторно использовать логин после soft delete.
CREATE UNIQUE INDEX idx_users_username ON users (username) WHERE deleted_at IS NULL;
-- Ускоряет выборки «только активные» и «только удалённые» пользователи.
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

-- +goose Down
-- Откат: удаляем таблицу пользователей целиком (данные будут потеряны).
DROP TABLE IF EXISTS users;

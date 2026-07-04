-- +goose Up
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_users_username ON users (username) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

-- +goose Down
DROP TABLE IF EXISTS users;

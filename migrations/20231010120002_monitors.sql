-- +goose Up
CREATE TABLE monitor_urls (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    name TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL,
    is_up BOOLEAN,
    last_checked_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_monitor_urls_deleted_at ON monitor_urls (deleted_at);

-- +goose Down
DROP TABLE IF EXISTS monitor_urls;

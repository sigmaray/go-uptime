-- +goose Up
CREATE TABLE app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_settings (key, value) VALUES ('check_interval_seconds', '60');

-- +goose Down
DROP TABLE IF EXISTS app_settings;

-- +goose Up
ALTER TABLE monitor_urls
    ADD COLUMN notify_telegram BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN notify_smtp BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE monitor_urls
    DROP COLUMN IF EXISTS notify_telegram,
    DROP COLUMN IF EXISTS notify_smtp;

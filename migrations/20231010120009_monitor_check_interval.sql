-- +goose Up
ALTER TABLE monitor_urls
    ADD COLUMN check_interval_seconds INTEGER;

-- +goose Down
ALTER TABLE monitor_urls
    DROP COLUMN IF EXISTS check_interval_seconds;

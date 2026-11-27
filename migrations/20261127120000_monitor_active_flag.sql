-- +goose Up
ALTER TABLE monitor_urls
    RENAME COLUMN is_active TO active;

ALTER TABLE monitor_urls
    DROP COLUMN lifecycle_version,
    DROP COLUMN uptime_baseline_at;

-- +goose Down
ALTER TABLE monitor_urls
    ADD COLUMN lifecycle_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN uptime_baseline_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE monitor_urls
    RENAME COLUMN active TO is_active;

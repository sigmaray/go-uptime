-- +goose Up
ALTER TABLE monitor_urls
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN lifecycle_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN uptime_baseline_at TIMESTAMP WITH TIME ZONE;

-- +goose Down
ALTER TABLE monitor_urls
    DROP COLUMN uptime_baseline_at,
    DROP COLUMN lifecycle_version,
    DROP COLUMN is_active;

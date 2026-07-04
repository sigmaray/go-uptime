-- +goose Up
CREATE TABLE stat_minutely (
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    bucket_at TIMESTAMPTZ NOT NULL,
    up_seconds INTEGER NOT NULL DEFAULT 0,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_url_id, bucket_at)
);

CREATE INDEX idx_stat_minutely_bucket_at ON stat_minutely (bucket_at);

CREATE TABLE stat_hourly (
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    bucket_at TIMESTAMPTZ NOT NULL,
    up_seconds INTEGER NOT NULL DEFAULT 0,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_url_id, bucket_at)
);

CREATE INDEX idx_stat_hourly_bucket_at ON stat_hourly (bucket_at);

CREATE TABLE stat_daily (
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    bucket_at TIMESTAMPTZ NOT NULL,
    up_seconds INTEGER NOT NULL DEFAULT 0,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_url_id, bucket_at)
);

CREATE INDEX idx_stat_daily_bucket_at ON stat_daily (bucket_at);

-- +goose Down
DROP TABLE IF EXISTS stat_daily;
DROP TABLE IF EXISTS stat_hourly;
DROP TABLE IF EXISTS stat_minutely;

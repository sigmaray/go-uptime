-- +goose Up
CREATE TABLE monitor_checks (
    id BIGSERIAL PRIMARY KEY,
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    checked_at TIMESTAMPTZ NOT NULL,
    is_up BOOLEAN NOT NULL
);

CREATE INDEX idx_monitor_checks_monitor_checked ON monitor_checks (monitor_url_id, checked_at DESC);
CREATE INDEX idx_monitor_checks_checked_at ON monitor_checks (checked_at);

-- +goose Down
DROP TABLE IF EXISTS monitor_checks;

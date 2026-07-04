-- +goose Up
CREATE TABLE incidents (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_incidents_monitor_url_id ON incidents (monitor_url_id);
CREATE INDEX idx_incidents_resolved_at ON incidents (resolved_at);
CREATE INDEX idx_incidents_started_at ON incidents (started_at);

-- +goose Down
DROP TABLE IF EXISTS incidents;

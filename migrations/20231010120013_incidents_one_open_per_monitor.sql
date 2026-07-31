-- +goose Up
-- Resolve duplicate open incidents before enforcing one-open-per-monitor.
WITH ranked AS (
    SELECT
        id,
        row_number() OVER (
            PARTITION BY monitor_url_id
            ORDER BY started_at ASC, id ASC
        ) AS rn
    FROM incidents
    WHERE resolved_at IS NULL
)
UPDATE incidents
SET resolved_at = NOW(),
    updated_at = NOW()
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX idx_incidents_one_open_per_monitor
    ON incidents (monitor_url_id)
    WHERE resolved_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_incidents_one_open_per_monitor;

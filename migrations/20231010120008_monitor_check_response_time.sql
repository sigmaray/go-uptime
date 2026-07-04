-- +goose Up
ALTER TABLE monitor_checks ADD COLUMN response_time_ms INTEGER;

-- +goose Down
ALTER TABLE monitor_checks DROP COLUMN IF EXISTS response_time_ms;

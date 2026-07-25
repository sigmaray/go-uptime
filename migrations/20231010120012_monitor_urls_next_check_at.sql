-- +goose Up
ALTER TABLE monitor_urls ADD COLUMN next_check_at TIMESTAMP WITH TIME ZONE;
CREATE INDEX idx_monitor_urls_next_check_at ON monitor_urls(next_check_at);

-- +goose Down
DROP INDEX IF EXISTS idx_monitor_urls_next_check_at;
ALTER TABLE monitor_urls DROP COLUMN next_check_at;

-- +goose Up
DELETE FROM monitor_urls WHERE deleted_at IS NOT NULL;
DROP INDEX IF EXISTS idx_monitor_urls_deleted_at;
ALTER TABLE monitor_urls DROP COLUMN IF EXISTS deleted_at;

-- +goose Down
ALTER TABLE monitor_urls ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_monitor_urls_deleted_at ON monitor_urls (deleted_at);

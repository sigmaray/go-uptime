-- +goose Up
-- Remove duplicate URLs (keep the oldest row) before enforcing uniqueness.
DELETE FROM monitor_urls a
    USING monitor_urls b
WHERE a.url = b.url
  AND a.id > b.id;

CREATE UNIQUE INDEX idx_monitor_urls_url ON monitor_urls (url);

-- +goose Down
DROP INDEX IF EXISTS idx_monitor_urls_url;

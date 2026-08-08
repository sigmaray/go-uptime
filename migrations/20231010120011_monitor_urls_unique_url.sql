-- +goose Up
-- Гарантия: один URL — один монитор в системе. Дубликаты ломают расписание и уведомления.

-- Очистка данных: удалить дубликаты URL, оставив строку с наименьшим id (самую старую запись).
DELETE FROM monitor_urls a
    USING monitor_urls b
WHERE a.url = b.url
  AND a.id > b.id;

-- Уникальный индекс по url: новые INSERT/UPDATE с тем же URL будут отклонены БД.
CREATE UNIQUE INDEX idx_monitor_urls_url ON monitor_urls (url);

-- +goose Down
-- Откат: снимаем ограничение уникальности URL (дубликаты снова возможны).
DROP INDEX IF EXISTS idx_monitor_urls_url;

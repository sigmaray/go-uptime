-- +goose Up
-- Отказ от soft delete для мониторов: удаление становится физическим (DELETE), упрощается модель и уникальность URL.
-- Перед удалением колонки нужно убрать «мёртвые» строки и индекс по deleted_at.

-- Физически удалить все ранее помеченные deleted_at мониторы (каскад затронет связанные таблицы).
DELETE FROM monitor_urls WHERE deleted_at IS NOT NULL;
-- Индекс больше не нужен — колонка deleted_at будет удалена.
DROP INDEX IF EXISTS idx_monitor_urls_deleted_at;
-- Убираем колонку мягкого удаления; дальше DELETE FROM monitor_urls = окончательное удаление.
ALTER TABLE monitor_urls DROP COLUMN IF EXISTS deleted_at;

-- +goose Down
-- Откат: восстанавливаем soft delete для monitor_urls (данные уже удалённые не вернутся).
ALTER TABLE monitor_urls ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_monitor_urls_deleted_at ON monitor_urls (deleted_at);

-- +goose Up
-- Время ответа HTTP-проверки в миллисекундах; NULL — если замер не выполнялся или недоступен.
-- Позволяет строить графики latency и диагностировать «медленно, но up».
ALTER TABLE monitor_checks ADD COLUMN response_time_ms INTEGER;

-- +goose Down
-- Откат: удаляем колонку времени ответа из журнала проверок.
ALTER TABLE monitor_checks DROP COLUMN IF EXISTS response_time_ms;

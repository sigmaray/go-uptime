-- +goose Up
-- Индивидуальный интервал проверки монитора в секундах; NULL = использовать глобальное app_settings.check_interval_seconds.
-- Даёт разным URL разную частоту опроса без изменения настроек всего приложения.
ALTER TABLE monitor_urls
    ADD COLUMN check_interval_seconds INTEGER;

-- +goose Down
-- Откат: мониторы снова полагаются только на глобальный интервал из app_settings.
ALTER TABLE monitor_urls
    DROP COLUMN IF EXISTS check_interval_seconds;

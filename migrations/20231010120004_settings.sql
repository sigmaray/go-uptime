-- +goose Up
-- Таблица глобальных настроек приложения: пары ключ–значение (key-value store).
-- Используется для параметров, редактируемых через UI/API без перезапуска (интервал проверки и т.д.).
CREATE TABLE app_settings (
    -- Уникальный идентификатор настройки (например, check_interval_seconds).
    key TEXT PRIMARY KEY,
    -- Значение в текстовом виде; приложение парсит тип при чтении.
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Начальное значение: интервал проверки мониторов по умолчанию — 60 секунд (если у монитора нет своего).
INSERT INTO app_settings (key, value) VALUES ('check_interval_seconds', '60');

-- +goose Down
-- Откат: удаляем таблицу настроек вместе со всеми значениями.
DROP TABLE IF EXISTS app_settings;

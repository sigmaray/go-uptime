-- +goose Up
-- Таблица мониторов (monitor_urls): URL или endpoint, который периодически проверяется воркером.
-- Хранит текущий статус (is_up), время последней проверки и текст последней ошибки.
CREATE TABLE monitor_urls (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Мягкое удаление: монитор скрыт из UI, но строка остаётся до явной очистки (см. миграцию 20231010120010).
    deleted_at TIMESTAMPTZ,
    -- Человекочитаемое имя монитора в списке (может быть пустым).
    name TEXT NOT NULL DEFAULT '',
    -- Адрес для HTTP-проверки; позже станет уникальным (миграция 20231010120011).
    url TEXT NOT NULL,
    -- Текущий статус: true = доступен, false = недоступен, NULL = ещё не проверяли.
    is_up BOOLEAN,
    -- Момент последней успешной или неуспешной проверки воркером.
    last_checked_at TIMESTAMPTZ,
    -- Текст ошибки последней неудачной проверки (пустая строка, если ошибки нет).
    last_error TEXT NOT NULL DEFAULT ''
);

-- Фильтрация активных/удалённых мониторов в списках и воркере.
CREATE INDEX idx_monitor_urls_deleted_at ON monitor_urls (deleted_at);

-- +goose Down
-- Откат: удаляем таблицу мониторов (каскадно затронет incidents, monitor_checks, stat_*).
DROP TABLE IF EXISTS monitor_urls;

-- +goose Up
-- Таблица инцидентов: периоды недоступности монитора от started_at до resolved_at.
-- Один инцидент = одна непрерывная «поломка»; закрывается, когда монитор снова up.
CREATE TABLE incidents (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Связь с монитором; при удалении монитора инциденты удаляются каскадом.
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    -- Момент начала простоя (первая неудачная проверка после серии успехов).
    started_at TIMESTAMPTZ NOT NULL,
    -- NULL = инцидент ещё открыт (монитор down); NOT NULL = восстановление зафиксировано.
    resolved_at TIMESTAMPTZ,
    -- Сообщение об ошибке на момент открытия/в течение инцидента.
    error_message TEXT NOT NULL DEFAULT ''
);

-- История инцидентов конкретного монитора (страница монитора, уведомления).
CREATE INDEX idx_incidents_monitor_url_id ON incidents (monitor_url_id);
-- Быстрый поиск открытых инцидентов (resolved_at IS NULL) и фильтр «только закрытые».
CREATE INDEX idx_incidents_resolved_at ON incidents (resolved_at);
-- Сортировка и отчёты по времени начала простоя.
CREATE INDEX idx_incidents_started_at ON incidents (started_at);

-- +goose Down
-- Откат: удаляем таблицу инцидентов.
DROP TABLE IF EXISTS incidents;

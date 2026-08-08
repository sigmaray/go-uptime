-- +goose Up
-- Журнал отдельных проверок монитора: каждая строка — один HTTP-запрос воркера в момент checked_at.
-- Нужен для графиков, истории и агрегации в stat_* (uptime по минутам/часам/дням).
CREATE TABLE monitor_checks (
    id BIGSERIAL PRIMARY KEY,
    -- К какому монитору относится проверка; каскадное удаление при удалении монитора.
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    -- Время выполнения проверки (не путать с created_at таблицы мониторов).
    checked_at TIMESTAMPTZ NOT NULL,
    -- Результат: true = ответ считается успешным, false = down или ошибка.
    is_up BOOLEAN NOT NULL
);

-- Основной запрос: «последние N проверок монитора X» — сортировка по checked_at DESC.
CREATE INDEX idx_monitor_checks_monitor_checked ON monitor_checks (monitor_url_id, checked_at DESC);
-- Очистка старых записей, агрегация и отчёты по временному диапазону.
CREATE INDEX idx_monitor_checks_checked_at ON monitor_checks (checked_at);

-- +goose Down
-- Откат: удаляем журнал проверок.
DROP TABLE IF EXISTS monitor_checks;

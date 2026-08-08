-- +goose Up
-- Агрегированная статистика uptime: секунды «up» и «total» в фиксированных временных корзинах (buckets).
-- Данные накапливаются воркером из monitor_checks; три уровня детализации для графиков и отчётов.

-- Поминутная детализация: bucket_at — начало минуты (UTC); up_seconds/total_seconds — накопление за минуту.
CREATE TABLE stat_minutely (
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    bucket_at TIMESTAMPTZ NOT NULL,
    up_seconds INTEGER NOT NULL DEFAULT 0,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_url_id, bucket_at)
);

-- Выборка статистики по периоду (графики за последние часы/дни на минутной сетке).
CREATE INDEX idx_stat_minutely_bucket_at ON stat_minutely (bucket_at);

-- Почасовая агрегация: те же поля, bucket_at — начало часа; меньше строк для длинных периодов.
CREATE TABLE stat_hourly (
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    bucket_at TIMESTAMPTZ NOT NULL,
    up_seconds INTEGER NOT NULL DEFAULT 0,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_url_id, bucket_at)
);

CREATE INDEX idx_stat_hourly_bucket_at ON stat_hourly (bucket_at);

-- Дневная агрегация: bucket_at — начало календарного дня; для долгосрочного uptime и SLA.
CREATE TABLE stat_daily (
    monitor_url_id BIGINT NOT NULL REFERENCES monitor_urls (id) ON DELETE CASCADE,
    bucket_at TIMESTAMPTZ NOT NULL,
    up_seconds INTEGER NOT NULL DEFAULT 0,
    total_seconds INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_url_id, bucket_at)
);

CREATE INDEX idx_stat_daily_bucket_at ON stat_daily (bucket_at);

-- +goose Down
-- Откат: удаляем таблицы статистики (от дневной к поминутной — порядок не критичен для DROP).
DROP TABLE IF EXISTS stat_daily;
DROP TABLE IF EXISTS stat_hourly;
DROP TABLE IF EXISTS stat_minutely;

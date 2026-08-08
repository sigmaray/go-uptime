-- +goose Up
-- Инвариант: у каждого монитора не более одного открытого инцидента (resolved_at IS NULL).
-- Упрощает логику воркера и UI: «текущий простой» однозначен.

-- Подготовка данных: если уже несколько открытых инцидентов на один monitor_url_id,
-- оставляем самый ранний (started_at, затем id), остальные принудительно закрываем.
WITH ranked AS (
    SELECT
        id,
        row_number() OVER (
            PARTITION BY monitor_url_id
            ORDER BY started_at ASC, id ASC
        ) AS rn
    FROM incidents
    WHERE resolved_at IS NULL
)
UPDATE incidents
SET resolved_at = NOW(),
    updated_at = NOW()
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

-- Частичный уникальный индекс: только для открытых инцидентов — второй open на тот же monitor_url_id запрещён.
CREATE UNIQUE INDEX idx_incidents_one_open_per_monitor
    ON incidents (monitor_url_id)
    WHERE resolved_at IS NULL;

-- +goose Down
-- Откат: снимаем ограничение «один открытый инцидент на монитор» (историю закрытых дубликатов не восстанавливаем).
DROP INDEX IF EXISTS idx_incidents_one_open_per_monitor;

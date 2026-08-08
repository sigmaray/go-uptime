-- +goose Up
-- Планирование проверок: когда воркер должен следующий раз опросить этот монитор.
-- NULL = «к проверке сейчас» или ещё не рассчитано; после проверки воркер сдвигает next_check_at вперёд.

ALTER TABLE monitor_urls ADD COLUMN next_check_at TIMESTAMP WITH TIME ZONE;
-- Индекс для выборки «мониторы, у которых next_check_at <= now()» — очередь воркера без full scan.
CREATE INDEX idx_monitor_urls_next_check_at ON monitor_urls(next_check_at);

-- +goose Down
-- Откат: убираем планировщик next_check_at (воркер может вернуться к другой стратегии выборки).
DROP INDEX IF EXISTS idx_monitor_urls_next_check_at;
ALTER TABLE monitor_urls DROP COLUMN next_check_at;

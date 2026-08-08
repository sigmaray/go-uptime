-- +goose Up
-- Флаги уведомлений на уровне монитора: включать ли Telegram и/или SMTP при смене статуса или инциденте.
-- Глобальные credentials задаются отдельно (настройки приложения); здесь только «слать ли для этого URL».
ALTER TABLE monitor_urls
    -- true = отправлять уведомления в Telegram при событиях по этому монитору.
    ADD COLUMN notify_telegram BOOLEAN NOT NULL DEFAULT false,
    -- true = отправлять email через SMTP при событиях по этому монитору.
    ADD COLUMN notify_smtp BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
-- Откат: убираем колонки флагов уведомлений на уровне каждого монитора.
ALTER TABLE monitor_urls
    DROP COLUMN IF EXISTS notify_telegram,
    DROP COLUMN IF EXISTS notify_smtp;

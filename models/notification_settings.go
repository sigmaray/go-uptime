package models

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	// SettingNotificationTelegramURL — ключ app_settings для Telegram Shoutrrr URL.
	SettingNotificationTelegramURL = "notification_telegram_url"
	// SettingNotificationSMTPHost — ключ app_settings для SMTP-хоста.
	SettingNotificationSMTPHost = "notification_smtp_host"
	// SettingNotificationSMTPPort — ключ app_settings для SMTP-порта.
	SettingNotificationSMTPPort = "notification_smtp_port"
	// SettingNotificationSMTPUser — ключ app_settings для имени пользователя SMTP.
	SettingNotificationSMTPUser = "notification_smtp_user"
	// SettingNotificationSMTPPassword — ключ app_settings для пароля SMTP.
	SettingNotificationSMTPPassword = "notification_smtp_password"
	// SettingNotificationSMTPFrom — ключ app_settings для адреса отправителя SMTP.
	SettingNotificationSMTPFrom = "notification_smtp_from"
	// SettingNotificationSMTPTo — ключ app_settings для адреса получателя SMTP.
	SettingNotificationSMTPTo = "notification_smtp_to"
)

// NotificationSettings хранит параметры каналов уведомлений из app_settings.
type NotificationSettings struct {
	TelegramURL  string
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPTo       string
}

// TelegramConfigured сообщает, настроен ли Telegram Shoutrrr URL.
func (s NotificationSettings) TelegramConfigured() bool {
	return strings.TrimSpace(s.TelegramURL) != ""
}

// SMTPConfigured сообщает, заданы ли обязательные параметры SMTP.
func (s NotificationSettings) SMTPConfigured() bool {
	return strings.TrimSpace(s.SMTPHost) != "" &&
		s.SMTPPort > 0 &&
		strings.TrimSpace(s.SMTPTo) != ""
}

// LoadNotificationSettings читает настройки уведомлений из app_settings.
// db — подключение GORM к PostgreSQL.
func LoadNotificationSettings(db *gorm.DB) (NotificationSettings, error) {
	keys := []string{
		SettingNotificationTelegramURL,
		SettingNotificationSMTPHost,
		SettingNotificationSMTPPort,
		SettingNotificationSMTPUser,
		SettingNotificationSMTPPassword,
		SettingNotificationSMTPFrom,
		SettingNotificationSMTPTo,
	}

	var rows []AppSetting
	if err := db.Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return NotificationSettings{}, err
	}

	// Собираем map key→value для удобного доступа по константам Setting*.
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}

	port, _ := strconv.Atoi(values[SettingNotificationSMTPPort])
	if port <= 0 {
		// Порт не задан или невалиден — дефолт submission port 587.
		port = 587
	}

	return NotificationSettings{
		TelegramURL:  values[SettingNotificationTelegramURL],
		SMTPHost:     values[SettingNotificationSMTPHost],
		SMTPPort:     port,
		SMTPUser:     values[SettingNotificationSMTPUser],
		SMTPPassword: values[SettingNotificationSMTPPassword],
		SMTPFrom:     values[SettingNotificationSMTPFrom],
		SMTPTo:       values[SettingNotificationSMTPTo],
	}, nil
}

// SaveNotificationSettings сохраняет настройки уведомлений в app_settings.
// db — подключение GORM; settings содержит новые значения; keepSMTPPassword
// сохраняет существующий пароль, когда settings.SMTPPassword — пустая строка.
func SaveNotificationSettings(db *gorm.DB, settings NotificationSettings, keepSMTPPassword bool) error {
	if keepSMTPPassword && strings.TrimSpace(settings.SMTPPassword) == "" {
		// Форма не отправила пароль — оставляем сохранённый в БД (masked field в UI).
		existing, err := LoadNotificationSettings(db)
		if err != nil {
			return err
		}
		settings.SMTPPassword = existing.SMTPPassword
	}

	entries := map[string]string{
		SettingNotificationTelegramURL:  strings.TrimSpace(settings.TelegramURL),
		SettingNotificationSMTPHost:     strings.TrimSpace(settings.SMTPHost),
		SettingNotificationSMTPPort:     strconv.Itoa(settings.SMTPPort),
		SettingNotificationSMTPUser:     strings.TrimSpace(settings.SMTPUser),
		SettingNotificationSMTPPassword: settings.SMTPPassword,
		SettingNotificationSMTPFrom:     strings.TrimSpace(settings.SMTPFrom),
		SettingNotificationSMTPTo:       strings.TrimSpace(settings.SMTPTo),
	}

	for key, value := range entries {
		setting := AppSetting{Key: key, Value: value}
		// db.Save — upsert по primary key (key).
		if err := db.Save(&setting).Error; err != nil {
			return err
		}
	}

	return nil
}

// BuildSMTPShoutrrrURL формирует Shoutrrr SMTP URL из отдельных полей настроек.
// settings содержит параметры SMTP; subject — тема письма (может быть пустой).
func BuildSMTPShoutrrrURL(settings NotificationSettings, subject string) (string, error) {
	if !settings.SMTPConfigured() {
		return "", fmt.Errorf("smtp settings are incomplete")
	}

	// Shoutrrr ожидает credentials в userinfo URL: smtp://user:pass@host:port?...
	userInfo := url.UserPassword(settings.SMTPUser, settings.SMTPPassword)
	serviceURL := &url.URL{
		Scheme: "smtp",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort),
	}

	query := url.Values{}
	if settings.SMTPFrom != "" {
		query.Set("from", settings.SMTPFrom)
	}
	query.Set("to", settings.SMTPTo)
	if subject != "" {
		query.Set("subject", subject)
	}
	serviceURL.RawQuery = query.Encode()

	return serviceURL.String(), nil
}

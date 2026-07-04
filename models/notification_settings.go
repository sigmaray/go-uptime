package models

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	// SettingNotificationTelegramURL is the app_settings key for the Telegram Shoutrrr URL.
	SettingNotificationTelegramURL = "notification_telegram_url"
	// SettingNotificationSMTPHost is the app_settings key for the SMTP host.
	SettingNotificationSMTPHost = "notification_smtp_host"
	// SettingNotificationSMTPPort is the app_settings key for the SMTP port.
	SettingNotificationSMTPPort = "notification_smtp_port"
	// SettingNotificationSMTPUser is the app_settings key for the SMTP username.
	SettingNotificationSMTPUser = "notification_smtp_user"
	// SettingNotificationSMTPPassword is the app_settings key for the SMTP password.
	SettingNotificationSMTPPassword = "notification_smtp_password"
	// SettingNotificationSMTPFrom is the app_settings key for the SMTP sender address.
	SettingNotificationSMTPFrom = "notification_smtp_from"
	// SettingNotificationSMTPTo is the app_settings key for the SMTP recipient address.
	SettingNotificationSMTPTo = "notification_smtp_to"
)

// NotificationSettings holds notification channel parameters from app_settings.
type NotificationSettings struct {
	TelegramURL  string
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPTo       string
}

// TelegramConfigured reports whether a Telegram Shoutrrr URL is configured.
func (s NotificationSettings) TelegramConfigured() bool {
	return strings.TrimSpace(s.TelegramURL) != ""
}

// SMTPConfigured reports whether required SMTP parameters are set.
func (s NotificationSettings) SMTPConfigured() bool {
	return strings.TrimSpace(s.SMTPHost) != "" &&
		s.SMTPPort > 0 &&
		strings.TrimSpace(s.SMTPTo) != ""
}

// LoadNotificationSettings reads notification settings from app_settings.
// db is the GORM connection to PostgreSQL.
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

	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}

	port, _ := strconv.Atoi(values[SettingNotificationSMTPPort])
	if port <= 0 {
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

// SaveNotificationSettings saves notification settings to app_settings.
// db is the GORM connection; settings holds the new values; keepSMTPPassword retains
// the existing password when settings.SMTPPassword is an empty string.
func SaveNotificationSettings(db *gorm.DB, settings NotificationSettings, keepSMTPPassword bool) error {
	if keepSMTPPassword && strings.TrimSpace(settings.SMTPPassword) == "" {
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
		if err := db.Save(&setting).Error; err != nil {
			return err
		}
	}

	return nil
}

// BuildSMTPShoutrrrURL builds a Shoutrrr SMTP URL from individual settings fields.
// settings holds SMTP parameters; subject is the email subject (may be empty).
func BuildSMTPShoutrrrURL(settings NotificationSettings, subject string) (string, error) {
	if !settings.SMTPConfigured() {
		return "", fmt.Errorf("smtp settings are incomplete")
	}

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

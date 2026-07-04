package models

import (
	"errors"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	en "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	"gorm.io/gorm"
)

var (
	validate *validator.Validate
	trans    ut.Translator
)

func init() {
	validate = validator.New()

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("label"), ",", 2)[0]
		if name != "" {
			return name
		}
		return fld.Name
	})

	_ = validate.RegisterValidation("monitor_url", validateMonitorURL)
	_ = validate.RegisterValidation("telegram_shoutrrr_url", validateTelegramShoutrrrURL)

	english := en.New()
	uni := ut.New(english, english)
	trans, _ = uni.GetTranslator("en")
	_ = en_translations.RegisterDefaultTranslations(validate, trans)
}

// validateTelegramShoutrrrURL checks that the string is a non-empty Shoutrrr URL for Telegram.
func validateTelegramShoutrrrURL(fl validator.FieldLevel) bool {
	raw := strings.TrimSpace(fl.Field().String())
	if raw == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(raw), "telegram://")
}

// validateMonitorURL checks that the URL uses the http or https scheme.
func validateMonitorURL(fl validator.FieldLevel) bool {
	raw := fl.Field().String()
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// Validate validates CreateUserInput.
func (input CreateUserInput) Validate() error {
	return validate.Struct(input)
}

// Validate validates UpdateUserInput.
func (input UpdateUserInput) Validate() error {
	return validate.Struct(input)
}

// Validate validates MonitorURLInput.
func (input MonitorURLInput) Validate() error {
	return validate.Struct(input)
}

// Validate validates SettingsInput.
func (input SettingsInput) Validate() error {
	return validate.Struct(input)
}

// FormatValidationError converts validator errors into a human-readable string.
func FormatValidationError(err error) string {
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err.Error()
	}

	messages := make([]string, 0, len(validationErrors))
	for _, fieldErr := range validationErrors {
		messages = append(messages, fieldErr.Translate(trans))
	}
	return strings.Join(messages, "; ")
}

// GetCheckIntervalSeconds reads the check interval from the database or returns the default value.
func GetCheckIntervalSeconds(db *gorm.DB, defaultSeconds int) int {
	var setting AppSetting
	err := db.Where("key = ?", SettingCheckInterval).First(&setting).Error
	if err != nil {
		return defaultSeconds
	}
	seconds, err := strconv.Atoi(setting.Value)
	if err != nil || seconds < 10 {
		return defaultSeconds
	}
	return seconds
}

// SetCheckIntervalSeconds saves the check interval to the database.
func SetCheckIntervalSeconds(db *gorm.DB, seconds int) error {
	setting := AppSetting{
		Key:   SettingCheckInterval,
		Value: strconv.Itoa(seconds),
	}
	return db.Save(&setting).Error
}

// FindOpenIncident finds an open incident for the given URL.
func FindOpenIncident(db *gorm.DB, monitorURLID uint) (*Incident, error) {
	var incident Incident
	err := db.Where("monitor_url_id = ? AND resolved_at IS NULL", monitorURLID).First(&incident).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &incident, nil
}

// Package forms содержит DTO HTTP/CLI запросов и структурную валидацию.
// Модели персистентности остаются в пакете models; эти типы не являются сущностями GORM.
package forms

import (
	"errors"
	"net/url"
	"reflect"
	"strings"

	en "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

var (
	validate *validator.Validate
	trans    ut.Translator
)

// init настраивает глобальный валидатор go-playground/validator для всех DTO пакета forms.
// Теги validate:"..." на полях структур проверяются через validate.Struct (см. MonitorURLInput.Validate и аналоги).
// Тег label:"..." подставляется в сообщения об ошибках вместо имени поля Go.
func init() {
	validate = validator.New()

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("label"), ",", 2)[0]
		if name != "" {
			return name
		}
		return fld.Name
	})

	// Кастомные правила: monitor_url — только http/https; telegram_shoutrrr_url — префикс telegram:// (пустая строка допустима).
	_ = validate.RegisterValidation("monitor_url", validateMonitorURL)
	_ = validate.RegisterValidation("telegram_shoutrrr_url", validateTelegramShoutrrrURL)

	english := en.New()
	uni := ut.New(english, english)
	trans, _ = uni.GetTranslator("en")
	_ = en_translations.RegisterDefaultTranslations(validate, trans)
}

// validateTelegramShoutrrrURL проверяет, что строка — непустой Shoutrrr URL для Telegram.
// fl — уровень поля валидатора для проверяемой строки Telegram URL.
func validateTelegramShoutrrrURL(fl validator.FieldLevel) bool {
	raw := strings.TrimSpace(fl.Field().String())
	if raw == "" {
		// Пустой URL допустим — Telegram-канал необязателен.
		return true
	}
	return strings.HasPrefix(strings.ToLower(raw), "telegram://")
}

// validateMonitorURL проверяет, что URL использует схему http или https.
// fl — уровень поля валидатора для проверяемой строки URL монитора.
func validateMonitorURL(fl validator.FieldLevel) bool {
	raw := fl.Field().String()
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	// Только http/https — не file:, javascript: и т.д.
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// FormatValidationError преобразует ошибки валидатора в человекочитаемую строку.
// err — ошибка, возвращённая методом Validate или validate.Struct.
func FormatValidationError(err error) string {
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err.Error()
	}

	messages := make([]string, 0, len(validationErrors))
	for _, fieldErr := range validationErrors {
		// Translate использует label:"..." из тегов struct.
		messages = append(messages, fieldErr.Translate(trans))
	}
	return strings.Join(messages, "; ")
}

// Package forms holds HTTP/CLI request DTOs and structural validation.
// Persistence models stay in package models; these types are not GORM entities.
package forms

import (
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
// fl is the validator field level for the Telegram URL string being checked.
func validateTelegramShoutrrrURL(fl validator.FieldLevel) bool {
	raw := strings.TrimSpace(fl.Field().String())
	if raw == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(raw), "telegram://")
}

// validateMonitorURL checks that the URL uses the http or https scheme.
// fl is the validator field level for the monitor URL string being checked.
func validateMonitorURL(fl validator.FieldLevel) bool {
	raw := fl.Field().String()
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// FormatValidationError converts validator errors into a human-readable string.
// err is the error returned by a Validate method or validate.Struct.
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

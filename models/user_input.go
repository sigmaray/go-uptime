package models

import (
	"fmt"
	"strconv"
	"strings"
)

// CreateUserInput holds form data for creating a user.
type CreateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"required,min=8,max=128" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"required,eqfield=Password" label:"confirm password"`
}

// UpdateUserInput holds form data for editing a user.
type UpdateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"omitempty,min=8,max=128" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"eqfield=Password" label:"confirm password"`
}

// MonitorURLInput holds form data for creating or editing a monitored URL.
type MonitorURLInput struct {
	Name                 string `form:"name" validate:"omitempty,max=200" label:"name"`
	URL                  string `form:"url" validate:"required,url,monitor_url" label:"url"`
	CheckIntervalSeconds string `form:"check_interval_seconds" label:"check interval"`
	NotifyTelegram       bool   `form:"-"`
	NotifySMTP           bool   `form:"-"`
}

// ParseCheckIntervalSeconds converts the optional form field into a monitor-specific interval.
// An empty value means the monitor should inherit the global setting.
func (input MonitorURLInput) ParseCheckIntervalSeconds() (*int, error) {
	raw := strings.TrimSpace(input.CheckIntervalSeconds)
	if raw == "" {
		return nil, nil
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("check interval must be a whole number of seconds")
	}
	if seconds < 10 || seconds > 86400 {
		return nil, fmt.Errorf("check interval must be between 10 and 86400 seconds")
	}
	return &seconds, nil
}

// SettingsInput holds monitoring settings form data.
type SettingsInput struct {
	CheckIntervalSeconds int    `form:"check_interval_seconds" validate:"required,min=10,max=86400" label:"check interval"`
	TelegramURL          string `form:"notification_telegram_url" validate:"omitempty,telegram_shoutrrr_url" label:"telegram URL"`
	SMTPHost             string `form:"notification_smtp_host" validate:"omitempty,max=253" label:"smtp host"`
	SMTPPort             int    `form:"notification_smtp_port" validate:"omitempty,min=1,max=65535" label:"smtp port"`
	SMTPUser             string `form:"notification_smtp_user" validate:"omitempty,max=200" label:"smtp username"`
	SMTPPassword         string `form:"notification_smtp_password" validate:"omitempty,max=200" label:"smtp password"`
	SMTPFrom             string `form:"notification_smtp_from" validate:"omitempty,email" label:"smtp from"`
	SMTPTo               string `form:"notification_smtp_to" validate:"omitempty,email" label:"smtp to"`
}

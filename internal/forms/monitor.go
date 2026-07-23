package forms

import (
	"fmt"
	"strconv"
	"strings"
)

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

// Validate checks MonitorURLInput and the optional check-interval field.
func (input MonitorURLInput) Validate() error {
	if err := validate.Struct(input); err != nil {
		return err
	}
	_, err := input.ParseCheckIntervalSeconds()
	return err
}

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
	// VerifyBeforeCreate probes the URL before insert when true; unavailable sites are rejected.
	VerifyBeforeCreate bool `form:"verify_before_create"`
	NotifyTelegram     bool `form:"-"`
	NotifySMTP         bool `form:"-"`
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

// MonitorURLBulkInput holds form data for creating multiple monitored URLs at once.
// Name is not collected: each monitor's Name is set to its URL.
type MonitorURLBulkInput struct {
	URLs                 string `form:"urls"`
	CheckIntervalSeconds string `form:"check_interval_seconds" label:"check interval"`
	// VerifyBeforeCreate probes every URL before insert when true; unavailable sites reject the whole batch.
	VerifyBeforeCreate bool `form:"verify_before_create"`
	NotifyTelegram     bool `form:"-"`
	NotifySMTP         bool `form:"-"`
}

// ParseURLList splits raw into individual URLs by commas and newlines, trims whitespace,
// drops empty entries, and removes duplicates while preserving first-seen order.
// raw is the textarea content submitted by the user.
func ParseURLList(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", "\n")

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, part := range strings.Split(normalized, "\n") {
		url := strings.TrimSpace(part)
		if url == "" {
			continue
		}
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

// ParsedURLs returns the deduplicated URL list from the textarea field.
func (input MonitorURLBulkInput) ParsedURLs() []string {
	return ParseURLList(input.URLs)
}

// ParseCheckIntervalSeconds converts the optional form field into a monitor-specific interval.
// An empty value means each monitor should inherit the global setting.
func (input MonitorURLBulkInput) ParseCheckIntervalSeconds() (*int, error) {
	return MonitorURLInput{CheckIntervalSeconds: input.CheckIntervalSeconds}.ParseCheckIntervalSeconds()
}

// Validate checks that at least one URL is present, each URL is a valid monitor URL,
// and the optional check-interval field is valid when provided.
func (input MonitorURLBulkInput) Validate() error {
	urls := input.ParsedURLs()
	if len(urls) == 0 {
		return fmt.Errorf("at least one URL is required")
	}
	for _, rawURL := range urls {
		single := MonitorURLInput{URL: rawURL}
		if err := validate.Struct(single); err != nil {
			return fmt.Errorf("%s: %s", rawURL, FormatValidationError(err))
		}
	}
	_, err := input.ParseCheckIntervalSeconds()
	return err
}

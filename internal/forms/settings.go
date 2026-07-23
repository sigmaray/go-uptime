package forms

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

// Validate checks SettingsInput against structural validation rules.
func (input SettingsInput) Validate() error {
	return validate.Struct(input)
}

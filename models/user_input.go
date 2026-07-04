package models

// CreateUserInput — данные формы создания пользователя.
type CreateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"required,min=1" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"required,eqfield=Password" label:"confirm password"`
}

// UpdateUserInput — данные формы редактирования пользователя.
type UpdateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"omitempty,min=1" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"eqfield=Password" label:"confirm password"`
}

// MonitorURLInput — данные формы создания/редактирования URL для мониторинга.
type MonitorURLInput struct {
	Name string `form:"name" validate:"omitempty,max=200" label:"name"`
	URL  string `form:"url" validate:"required,url,monitor_url" label:"url"`
}

// SettingsInput — данные формы настроек мониторинга.
type SettingsInput struct {
	CheckIntervalSeconds int `form:"check_interval_seconds" validate:"required,min=10,max=86400" label:"check interval"`
}

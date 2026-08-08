package forms

// CreateUserInput хранит данные формы создания пользователя.
type CreateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"required,min=8,max=128" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"required,eqfield=Password" label:"confirm password"`
}

// Validate проверяет CreateUserInput по правилам структурной валидации.
func (input CreateUserInput) Validate() error {
	// eqfield=Password проверяет совпадение пароля и confirm_password.
	return validate.Struct(input)
}

// UpdateUserInput хранит данные формы редактирования пользователя.
type UpdateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"omitempty,min=8,max=128" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"eqfield=Password" label:"confirm password"`
}

// Validate проверяет UpdateUserInput по правилам структурной валидации.
func (input UpdateUserInput) Validate() error {
	// Password omitempty — пустой пароль означает «не менять»; confirm всё равно должен совпасть.
	return validate.Struct(input)
}

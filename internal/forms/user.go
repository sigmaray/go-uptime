package forms

// CreateUserInput holds form data for creating a user.
type CreateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"required,min=8,max=128" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"required,eqfield=Password" label:"confirm password"`
}

// Validate checks CreateUserInput against structural validation rules.
func (input CreateUserInput) Validate() error {
	return validate.Struct(input)
}

// UpdateUserInput holds form data for editing a user.
type UpdateUserInput struct {
	Username        string `form:"username" validate:"required,min=1,max=100" label:"login"`
	Password        string `form:"password" validate:"omitempty,min=8,max=128" label:"password"`
	ConfirmPassword string `form:"confirm_password" validate:"eqfield=Password" label:"confirm password"`
}

// Validate checks UpdateUserInput against structural validation rules.
func (input UpdateUserInput) Validate() error {
	return validate.Struct(input)
}

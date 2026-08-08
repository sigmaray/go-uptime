package models

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// HashPassword хеширует пароль с помощью bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword сравнивает пароль с bcrypt-хешем.
func CheckPassword(hash, password string) bool {
	// CompareHashAndPassword возвращает nil при совпадении.
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CreateUser создаёт нового пользователя с указанным именем и паролем.
// Deprecated: используйте repository.UserRepository.
func CreateUser(db *gorm.DB, username, password string) (User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}

	user := User{
		Username:     username,
		PasswordHash: hash,
	}

	if err := db.Create(&user).Error; err != nil {
		return User{}, err
	}

	return user, nil
}

// FindUserByUsername ищет пользователя по имени.
// Deprecated: используйте repository.UserRepository.
func FindUserByUsername(db *gorm.DB, username string) (*User, error) {
	var user User
	err := db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Пользователь не найден — не ошибка для login flow.
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

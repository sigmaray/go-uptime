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
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CreateUser создаёт нового пользователя с заданным логином и паролем.
func CreateUser(db *gorm.DB, username, password string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := User{
		Username:     username,
		PasswordHash: hash,
	}

	if err := db.Create(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// FindUserByUsername ищет пользователя по логину.
func FindUserByUsername(db *gorm.DB, username string) (*User, error) {
	var user User
	err := db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

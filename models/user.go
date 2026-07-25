package models

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// HashPassword hashes a password with bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a password with a bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CreateUser creates a new user with the given username and password.
// db is the GORM database handle.
// username is the unique login name to store.
// password is the plain-text password that will be bcrypt-hashed before save.
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

// FindUserByUsername looks up a user by username.
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

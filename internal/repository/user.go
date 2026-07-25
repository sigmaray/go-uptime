package repository

import (
	"errors"
	"go-uptime/models"
	"gorm.io/gorm"
)

// UserRepository handles database operations for User entities.
type UserRepository interface {
	// FindUserByUsername looks up a user by their unique username.
	// Returns nil, nil if the user does not exist.
	FindUserByUsername(username string) (*models.User, error)
	// CreateUser creates a new user with a securely hashed password.
	CreateUser(username, password string) (models.User, error)
}

// userRepository implements UserRepository using GORM.
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new repository for users.
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) FindUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) CreateUser(username, password string) (models.User, error) {
	hash, err := models.HashPassword(password)
	if err != nil {
		return models.User{}, err
	}

	user := models.User{
		Username:     username,
		PasswordHash: hash,
	}

	if err := r.db.Create(&user).Error; err != nil {
		return models.User{}, err
	}

	return user, nil
}

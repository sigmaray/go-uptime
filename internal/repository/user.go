package repository

import (
	"errors"
	"go-uptime/models"
	"gorm.io/gorm"
)

// UserRepository выполняет операции с сущностями User в базе данных.
type UserRepository interface {
	// FindUserByUsername ищет пользователя по уникальному имени.
	// Возвращает nil, nil, если пользователь не существует.
	FindUserByUsername(username string) (*models.User, error)
	// CreateUser создаёт нового пользователя с безопасно хешированным паролем.
	CreateUser(username, password string) (models.User, error)
}

// userRepository реализует UserRepository через GORM.
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository создаёт новый репозиторий пользователей.
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) FindUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Отсутствие пользователя — нормальный исход для login.
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) CreateUser(username, password string) (models.User, error) {
	// bcrypt-хеш до INSERT — пароль в открытом виде в БД не попадает.
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

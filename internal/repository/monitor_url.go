package repository

import (
	"go-uptime/models"
	"gorm.io/gorm"
)

// MonitorURLRepository handles database operations for MonitorURL entities.
type MonitorURLRepository interface {
	// FindDueMonitors returns monitors that haven't been checked yet or are past their next_check_at time.
	FindDueMonitors() ([]models.MonitorURL, error)
}

// monitorURLRepository implements MonitorURLRepository using GORM.
type monitorURLRepository struct {
	db *gorm.DB
}

// NewMonitorURLRepository creates a new repository for monitor URLs.
func NewMonitorURLRepository(db *gorm.DB) MonitorURLRepository {
	return &monitorURLRepository{db: db}
}

func (r *monitorURLRepository) FindDueMonitors() ([]models.MonitorURL, error) {
	var due []models.MonitorURL
	if err := r.db.Where("next_check_at IS NULL OR next_check_at <= NOW()").Find(&due).Error; err != nil {
		return nil, err
	}
	return due, nil
}

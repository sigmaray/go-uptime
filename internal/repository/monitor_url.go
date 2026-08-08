// Package repository содержит обёртки доступа к базе данных вокруг моделей GORM.
package repository

import (
	"go-uptime/models"
	"gorm.io/gorm"
)

// MonitorURLRepository выполняет операции с сущностями MonitorURL в базе данных.
type MonitorURLRepository interface {
	// FindDueMonitors возвращает мониторы, которые ещё не проверялись или у которых наступило next_check_at.
	FindDueMonitors() ([]models.MonitorURL, error)
}

// monitorURLRepository реализует MonitorURLRepository через GORM.
type monitorURLRepository struct {
	db *gorm.DB
}

// NewMonitorURLRepository создаёт новый репозиторий URL мониторов.
func NewMonitorURLRepository(db *gorm.DB) MonitorURLRepository {
	return &monitorURLRepository{db: db}
}

func (r *monitorURLRepository) FindDueMonitors() ([]models.MonitorURL, error) {
	var due []models.MonitorURL
	// next_check_at IS NULL — новый монитор ещё ни разу не claim-нут worker'ом.
	if err := r.db.Where("next_check_at IS NULL OR next_check_at <= NOW()").Find(&due).Error; err != nil {
		return nil, err
	}
	return due, nil
}

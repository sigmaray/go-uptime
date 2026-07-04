// Package models содержит GORM-модели и бизнес-логику работы с данными.
package models

import (
	"time"

	"gorm.io/gorm"
)

// User — учётная запись администратора.
type User struct {
	ID           uint `gorm:"primaryKey"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	Username     string         `gorm:"uniqueIndex;not null"`
	PasswordHash string         `gorm:"not null"`
}

// MonitorURL — HTTP/HTTPS ресурс для мониторинга.
type MonitorURL struct {
	ID             uint `gorm:"primaryKey"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	Name           string         `gorm:"not null;default:''"`
	URL            string         `gorm:"not null"`
	IsUp           *bool
	LastCheckedAt  *time.Time
	LastError      string `gorm:"not null;default:''"`
	NotifyTelegram bool   `gorm:"not null;default:false"`
	NotifySMTP     bool   `gorm:"not null;default:false"`
}

// Incident — период недоступности мониторируемого URL.
type Incident struct {
	ID           uint `gorm:"primaryKey"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MonitorURLID uint       `gorm:"not null;index"`
	MonitorURL   MonitorURL `gorm:"foreignKey:MonitorURLID"`
	StartedAt    time.Time  `gorm:"not null;index"`
	ResolvedAt   *time.Time `gorm:"index"`
	ErrorMessage string     `gorm:"not null;default:''"`
}

// IsOpen возвращает true, если инцидент ещё не закрыт.
func (i *Incident) IsOpen() bool {
	return i.ResolvedAt == nil
}

// AppSetting — пара ключ-значение для настроек приложения в БД.
type AppSetting struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
}

// SettingCheckInterval — ключ настройки интервала проверки в секундах.
const SettingCheckInterval = "check_interval_seconds"

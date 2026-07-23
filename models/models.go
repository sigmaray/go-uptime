// Package models contains GORM persistence models and database access helpers.
package models

import (
	"time"

	"gorm.io/gorm"
)

// User is an administrator account.
type User struct {
	ID           uint `gorm:"primaryKey"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	Username     string         `gorm:"uniqueIndex;not null"`
	PasswordHash string         `gorm:"not null"`
}

// MonitorURL is an HTTP/HTTPS resource to monitor.
type MonitorURL struct {
	ID                   uint `gorm:"primaryKey"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Name                 string `gorm:"not null;default:''"`
	URL                  string `gorm:"not null"`
	IsUp                 *bool
	LastCheckedAt        *time.Time
	LastError            string `gorm:"not null;default:''"`
	NotifyTelegram       bool   `gorm:"not null;default:false"`
	NotifySMTP           bool   `gorm:"not null;default:false"`
	CheckIntervalSeconds *int
}

// Incident is a period of downtime for a monitored URL.
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

// IsOpen returns true if the incident has not been resolved yet.
func (i *Incident) IsOpen() bool {
	return i.ResolvedAt == nil
}

// AppSetting is a key-value pair for application settings stored in the database.
type AppSetting struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
}

// SettingCheckInterval is the app_settings key for the check interval in seconds.
const SettingCheckInterval = "check_interval_seconds"

// DefaultCheckIntervalSeconds is used when no global check interval is stored in the database.
const DefaultCheckIntervalSeconds = 60

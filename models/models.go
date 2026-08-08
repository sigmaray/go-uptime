// Package models содержит модели персистентности GORM и вспомогательные функции доступа к базе данных.
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

// MonitorURL — HTTP/HTTPS-ресурс для мониторинга.
type MonitorURL struct {
	ID                   uint `gorm:"primaryKey"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Name                 string `gorm:"not null;default:''"`
	URL                  string `gorm:"uniqueIndex;not null"`
	// IsUp — последний известный статус: true=доступен, false=недоступен, nil=ещё ни разу не проверялся (новый монитор).
	IsUp          *bool
	LastCheckedAt *time.Time
	// NextCheckAt — когда worker должен снова claim-нуть монитор; индекс для выборки due-мониторов.
	// При claim сдвигается вперёд (lease), после успешной проверки — на now+interval.
	NextCheckAt   *time.Time `gorm:"index"`
	LastError     string     `gorm:"not null;default:''"`
	// NotifyTelegram — слать ли уведомления в Telegram при смене IsUp (требует глобальной настройки канала).
	NotifyTelegram bool `gorm:"not null;default:false"`
	// NotifySMTP — слать ли email при смене IsUp (требует глобальной SMTP-настройки).
	NotifySMTP           bool `gorm:"not null;default:false"`
	CheckIntervalSeconds *int
}

// Incident — период простоя отслеживаемого URL.
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

// IsOpen возвращает true, если инцидент ещё не разрешён.
func (i *Incident) IsOpen() bool {
	// Открытый инцидент — resolved_at ещё не проставлен worker'ом при UP.
	return i.ResolvedAt == nil
}

// AppSetting — пара ключ-значение для настроек приложения, хранящихся в базе данных.
type AppSetting struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
}

// SettingCheckInterval — ключ app_settings для интервала проверки в секундах.
const SettingCheckInterval = "check_interval_seconds"

// DefaultCheckIntervalSeconds используется, когда глобальный интервал проверки не сохранён в базе данных.
const DefaultCheckIntervalSeconds = 60

package models

import (
	"strconv"

	"gorm.io/gorm"
)

// GetCheckIntervalSeconds reads the global check interval from app_settings.
// db is the database handle used to load the setting.
// When the setting is missing or invalid, DefaultCheckIntervalSeconds is returned.
func GetCheckIntervalSeconds(db *gorm.DB) int {
	var setting AppSetting
	err := db.Where("key = ?", SettingCheckInterval).First(&setting).Error
	if err != nil {
		return DefaultCheckIntervalSeconds
	}
	seconds, err := strconv.Atoi(setting.Value)
	if err != nil || seconds < 10 {
		return DefaultCheckIntervalSeconds
	}
	return seconds
}

// SetCheckIntervalSeconds saves the global check interval to app_settings.
// db is the database handle used for persistence.
// seconds is the interval in seconds and must be validated by the caller.
func SetCheckIntervalSeconds(db *gorm.DB, seconds int) error {
	setting := AppSetting{
		Key:   SettingCheckInterval,
		Value: strconv.Itoa(seconds),
	}
	return db.Save(&setting).Error
}

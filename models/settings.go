package models

import (
	"strconv"

	"gorm.io/gorm"
)

// GetCheckIntervalSeconds читает глобальный интервал проверки из app_settings.
// db — дескриптор базы данных для загрузки настройки.
// Если настройка отсутствует или недействительна, возвращается DefaultCheckIntervalSeconds.
func GetCheckIntervalSeconds(db *gorm.DB) int {
	var setting AppSetting
	err := db.Where("key = ?", SettingCheckInterval).First(&setting).Error
	if err != nil {
		// Настройка не найдена — глобальный дефолт 60 секунд.
		return DefaultCheckIntervalSeconds
	}
	seconds, err := strconv.Atoi(setting.Value)
	if err != nil || seconds < 10 {
		// Битое значение в БД — откатываемся к дефолту.
		return DefaultCheckIntervalSeconds
	}
	return seconds
}

// SetCheckIntervalSeconds сохраняет глобальный интервал проверки в app_settings.
// db — дескриптор базы данных для персистентности.
// seconds — интервал в секундах; должен быть проверен вызывающим кодом.
func SetCheckIntervalSeconds(db *gorm.DB, seconds int) error {
	setting := AppSetting{
		Key:   SettingCheckInterval,
		Value: strconv.Itoa(seconds),
	}
	return db.Save(&setting).Error
}

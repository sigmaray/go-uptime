package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// CountIncidents возвращает общее число инцидентов по всем мониторам.
// db — дескриптор GORM для подсчёта строк инцидентов.
func CountIncidents(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Incident{}).Count(&count).Error
	return count, err
}

// CountOpenIncidents возвращает, сколько инцидентов ещё не разрешено.
// db — дескриптор GORM для подсчёта открытых инцидентов.
func CountOpenIncidents(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Incident{}).Where("resolved_at IS NULL").Count(&count).Error
	return count, err
}

// LoadIncidentsPage загружает одну страницу истории инцидентов, отсортированную от новых к старым.
//
// db — дескриптор базы данных для запроса.
// page — номер страницы, начиная с 1.
// perPage — сколько инцидентов содержит каждая страница.
func LoadIncidentsPage(db *gorm.DB, page, perPage int) ([]Incident, error) {
	if perPage < 1 {
		perPage = AdminListPageSize
	}
	if page < 1 {
		page = 1
	}

	var incidents []Incident
	// Preload MonitorURL — в таблице показываем имя/URL монитора без N+1.
	err := db.Preload("MonitorURL").
		Order("started_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&incidents).Error
	if err != nil {
		return nil, err
	}
	return incidents, nil
}

// CountIncidentsForMonitor возвращает, сколько инцидентов существует для монитора.
func CountIncidentsForMonitor(db *gorm.DB, monitorID uint) (int64, error) {
	var count int64
	err := db.Model(&Incident{}).Where("monitor_url_id = ?", monitorID).Count(&count).Error
	return count, err
}

// LoadIncidentsForMonitorPage загружает одну страницу истории инцидентов для монитора,
// отсортированную от новых к старым.
//
// db — дескриптор базы данных для запроса.
// monitorID — `monitor_urls.id`, для которого загружаются инциденты.
// page — номер страницы, начиная с 1.
// perPage — сколько инцидентов содержит каждая страница.
func LoadIncidentsForMonitorPage(db *gorm.DB, monitorID uint, page, perPage int) ([]Incident, error) {
	if perPage < 1 {
		perPage = MonitorDetailListPageSize
	}
	if page < 1 {
		page = 1
	}

	var incidents []Incident
	// Фильтр по monitor_url_id — только инциденты одного монитора на странице деталей.
	err := db.Where("monitor_url_id = ?", monitorID).
		Order("started_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&incidents).Error
	if err != nil {
		return nil, err
	}
	return incidents, nil
}

// FindOpenIncident находит открытый инцидент для указанного монитора.
// db — дескриптор базы данных для запроса.
// monitorURLID — monitor_urls.id, для которого загружается открытый инцидент.
// Возвращает nil, nil, если открытый инцидент отсутствует.
func FindOpenIncident(db *gorm.DB, monitorURLID uint) (*Incident, error) {
	var incident Incident
	// resolved_at IS NULL — единственный открытый инцидент на монитор (гарантия миграции/constraint).
	err := db.Where("monitor_url_id = ? AND resolved_at IS NULL", monitorURLID).First(&incident).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Нет открытого — не ошибка, caller создаст новый при DOWN.
			return nil, nil
		}
		return nil, err
	}
	return &incident, nil
}

// FindOpenIncidentsByMonitorIDs загружает открытые инциденты для многих мониторов одним запросом.
// db — дескриптор базы данных для запроса.
// monitorURLIDs — значения monitor_urls.id для поиска; пустой ввод возвращает пустую map.
// Возвращаемая map индексируется по monitor_url_id и содержит не более одного открытого инцидента на монитор.
func FindOpenIncidentsByMonitorIDs(db *gorm.DB, monitorURLIDs []uint) (map[uint]Incident, error) {
	out := make(map[uint]Incident, len(monitorURLIDs))
	if len(monitorURLIDs) == 0 {
		return out, nil
	}

	var incidents []Incident
	if err := db.Where("monitor_url_id IN ? AND resolved_at IS NULL", monitorURLIDs).Find(&incidents).Error; err != nil {
		return nil, err
	}
	// Ключ map — monitor_url_id; при constraint «один open» дубликатов не будет.
	for _, incident := range incidents {
		out[incident.MonitorURLID] = incident
	}
	return out, nil
}

// PruneIncidents удаляет старые разрешённые инциденты для ограничения роста данных.
// db — дескриптор базы данных для удалений.
// retentionDays определяет, как долго хранятся разрешённые инциденты.
// maxPerMonitor ограничивает, сколько разрешённых инцидентов может хранить каждый монитор.
func PruneIncidents(db *gorm.DB, retentionDays, maxPerMonitor int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	// Фаза 1: удалить разрешённые инциденты старше retentionDays по resolved_at.
	if err := db.Where("resolved_at IS NOT NULL AND resolved_at < ?", cutoff).
		Delete(&Incident{}).Error; err != nil {
		return err
	}

	if maxPerMonitor < 0 {
		maxPerMonitor = 0
	}
	// Фаза 2: для каждого монитора оставить только maxPerMonitor самых свежих разрешённых
	// (row_number по resolved_at DESC); остальные удалить, даже если ещё «молодые».
	return db.Exec(`
		WITH ranked AS (
			SELECT
				id,
				row_number() OVER (
					PARTITION BY monitor_url_id
					ORDER BY resolved_at DESC, id DESC
				) AS rn
			FROM incidents
			WHERE resolved_at IS NOT NULL
		)
		DELETE FROM incidents
		USING ranked
		WHERE incidents.id = ranked.id
			AND ranked.rn > ?
	`, maxPerMonitor).Error
}

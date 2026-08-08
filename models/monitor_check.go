package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	// monitorCheckRetention — минимальный возраст, после которого проверку можно удалить.
	monitorCheckRetention = 24 * time.Hour
	// maxMonitorChecksPerMonitor — сколько последних проверок всегда сохраняется для каждого монитора.
	maxMonitorChecksPerMonitor = 200
)

const (
	// MaxRecentHeartbeatsList — сколько heartbeat загружает глобальная страница списка.
	MaxRecentHeartbeatsList = 500
	// MaxMonitorDetailHeartbeats — сколько heartbeat загружает страница деталей монитора.
	MaxMonitorDetailHeartbeats = 200
	// HeartbeatHourMinutes — сколько одноминутных bucket покрывает heartbeat-график в админке.
	HeartbeatHourMinutes = 60
)

// HeartbeatMinuteCount — суммы успешных и неудачных heartbeat для одного минутного bucket.
type HeartbeatMinuteCount struct {
	// BucketAt — начало минуты (UTC, усечённое).
	BucketAt time.Time
	// Success — сколько heartbeat сообщили «up» за эту минуту.
	Success int64
	// Failed — сколько heartbeat сообщили «down» за эту минуту.
	Failed int64
}

// Total возвращает Success + Failed за минуту.
func (c HeartbeatMinuteCount) Total() int64 {
	return c.Success + c.Failed
}

// MonitorCheck хранит результат одной проверки HTTP-доступности.
type MonitorCheck struct {
	ID             uint       `gorm:"primaryKey"`
	MonitorURLID   uint       `gorm:"not null;index"`
	MonitorURL     MonitorURL `gorm:"foreignKey:MonitorURLID"`
	CheckedAt      time.Time
	IsUp           bool `gorm:"not null"`
	ResponseTimeMs *int
}

// RecordMonitorCheck сохраняет один результат проверки для истории uptime и обновляет агрегированную статистику.
// db — дескриптор базы данных для персистентности.
// monitorID — monitor_urls.id проверенного монитора.
// checkedAt — время завершения проверки.
// isUp — успешно ли ответила цель.
// responseTimeMs — необязательная измеренная задержка в миллисекундах.
func RecordMonitorCheck(db *gorm.DB, monitorID uint, checkedAt time.Time, isUp bool, responseTimeMs *int) error {
	// Сначала обновляем агрегированную uptime-статистику по интервалу с предыдущей проверкой.
	if err := UpdateUptimeStats(db, monitorID, checkedAt, isUp); err != nil {
		return err
	}

	check := MonitorCheck{
		MonitorURLID:   monitorID,
		CheckedAt:      checkedAt,
		IsUp:           isUp,
		ResponseTimeMs: responseTimeMs,
	}
	// Затем сохраняем сырую строку heartbeat для истории и графиков.
	return db.Create(&check).Error
}

// CountMonitorChecks возвращает общее число heartbeat по всем мониторам.
func CountMonitorChecks(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&MonitorCheck{}).Count(&count).Error
	return count, err
}

// LoadAllMonitorChecksPage загружает одну страницу heartbeat по всем мониторам, отсортированную от новых к старым.
//
// db — дескриптор базы данных для запроса.
// page — номер страницы, начиная с 1.
// perPage — сколько heartbeat содержит каждая страница.
func LoadAllMonitorChecksPage(db *gorm.DB, page, perPage int) ([]MonitorCheck, error) {
	if perPage < 1 {
		perPage = AdminListPageSize
	}
	if page < 1 {
		page = 1
	}

	var checks []MonitorCheck
	err := db.Preload("MonitorURL").
		Order("checked_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&checks).Error
	if err != nil {
		return nil, err
	}
	return checks, nil
}

// LoadRecentMonitorChecks возвращает самые последние heartbeat по всем мониторам.
// limit ограничивает число возвращаемых строк; значения выше MaxRecentHeartbeatsList усекаются.
func LoadRecentMonitorChecks(db *gorm.DB, limit int) ([]MonitorCheck, error) {
	if limit <= 0 || limit > MaxRecentHeartbeatsList {
		// Защита от слишком больших limit в query string.
		limit = MaxRecentHeartbeatsList
	}

	var checks []MonitorCheck
	if err := db.Preload("MonitorURL").Order("checked_at desc").Limit(limit).Find(&checks).Error; err != nil {
		return nil, err
	}
	return checks, nil
}

// LoadMonitorChecks возвращает heartbeat для одного монитора, отсортированные от новых к старым.
// limit ограничивает число возвращаемых строк; значения выше MaxMonitorDetailHeartbeats усекаются.
func LoadMonitorChecks(db *gorm.DB, monitorID uint, limit int) ([]MonitorCheck, error) {
	if limit <= 0 || limit > MaxMonitorDetailHeartbeats {
		limit = MaxMonitorDetailHeartbeats
	}

	var checks []MonitorCheck
	// checked_at DESC — последние heartbeat сверху на странице монитора.
	if err := db.Where("monitor_url_id = ?", monitorID).
		Order("checked_at desc").
		Limit(limit).
		Find(&checks).Error; err != nil {
		return nil, err
	}
	return checks, nil
}

// CountMonitorChecksForMonitor возвращает, сколько heartbeat существует для монитора.
func CountMonitorChecksForMonitor(db *gorm.DB, monitorID uint) (int64, error) {
	var count int64
	err := db.Model(&MonitorCheck{}).Where("monitor_url_id = ?", monitorID).Count(&count).Error
	return count, err
}

// LoadMonitorChecksPage загружает одну страницу heartbeat для монитора, отсортированную от новых к старым.
//
// db — дескриптор базы данных для запроса.
// monitorID — `monitor_urls.id`, для которого загружаются проверки.
// page — номер страницы, начиная с 1.
// perPage — сколько heartbeat содержит каждая страница.
func LoadMonitorChecksPage(db *gorm.DB, monitorID uint, page, perPage int) ([]MonitorCheck, error) {
	if perPage < 1 {
		perPage = MonitorDetailListPageSize
	}
	if page < 1 {
		page = 1
	}

	var checks []MonitorCheck
	err := db.Where("monitor_url_id = ?", monitorID).
		Order("checked_at desc").
		Offset(PageOffset(page, perPage)).
		Limit(perPage).
		Find(&checks).Error
	if err != nil {
		return nil, err
	}
	return checks, nil
}

// CountHeartbeatsByMinute агрегирует heartbeat в одноминутные bucket успехов и неудач.
// db — дескриптор базы данных для загрузки последних heartbeat.
// now — опорные часы; окно заканчивается на текущей усечённой минуте и охватывает HeartbeatHourMinutes.
// Возвращаемые строки включают только минуты с хотя бы одним heartbeat; вызывающий код заполняет пустые минуты.
func CountHeartbeatsByMinute(db *gorm.DB, now time.Time) ([]HeartbeatMinuteCount, error) {
	windowEnd := now.UTC().Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(HeartbeatHourMinutes-1) * time.Minute)
	// Полуинтервал [windowStart, until): until — exclusive, чтобы включить всю текущую минуту.
	until := windowEnd.Add(time.Minute)

	var checks []MonitorCheck
	// Загружаем только checked_at и is_up — меньше трафика из БД.
	if err := db.Select("checked_at", "is_up").
		Where("checked_at >= ? AND checked_at < ?", windowStart, until).
		Find(&checks).Error; err != nil {
		return nil, err
	}

	byMinute := make(map[int64]*HeartbeatMinuteCount, HeartbeatHourMinutes)
	for _, check := range checks {
		bucketAt := check.CheckedAt.UTC().Truncate(time.Minute)
		key := bucketAt.Unix()
		entry, ok := byMinute[key]
		if !ok {
			entry = &HeartbeatMinuteCount{BucketAt: bucketAt}
			byMinute[key] = entry
		}
		// Разносим heartbeat по счётчикам success/failed внутри минуты.
		if check.IsUp {
			entry.Success++
		} else {
			entry.Failed++
		}
	}

	counts := make([]HeartbeatMinuteCount, 0, len(byMinute))
	for _, entry := range byMinute {
		counts = append(counts, *entry)
	}
	// Разреженный результат: только минуты с ≥1 heartbeat; пустые минуты заполняет UI.
	return counts, nil
}

// LoadMonitorChecksSince группирует результаты проверок по ID монитора начиная с указанного времени.
func LoadMonitorChecksSince(db *gorm.DB, since time.Time) (map[uint][]MonitorCheck, error) {
	var checks []MonitorCheck
	if err := db.Where("checked_at >= ?", since).Order("checked_at asc").Find(&checks).Error; err != nil {
		return nil, err
	}

	// Группируем по monitor_url_id для пакетной обработки worker/maintenance.
	byMonitor := make(map[uint][]MonitorCheck, len(checks))
	for _, check := range checks {
		byMonitor[check.MonitorURLID] = append(byMonitor[check.MonitorURLID], check)
	}
	return byMonitor, nil
}

// PruneMonitorChecks удаляет проверки старше monitorCheckRetention, которые не входят
// в maxMonitorChecksPerMonitor самых последних проверок для своего монитора.
func PruneMonitorChecks(db *gorm.DB) error {
	cutoff := time.Now().Add(-monitorCheckRetention)

	// Удаляем только если выполнены ОБА условия: checked_at старше retention И проверка
	// не входит в maxMonitorChecksPerMonitor последних для своего монитора (rn > лимита).
	return db.Exec(`
		WITH ranked AS (
			SELECT
				id,
				row_number() OVER (
					PARTITION BY monitor_url_id
					ORDER BY checked_at DESC, id DESC
				) AS rn
			FROM monitor_checks
		)
		DELETE FROM monitor_checks
		USING ranked
		WHERE monitor_checks.id = ranked.id
			AND monitor_checks.checked_at < ?
			AND ranked.rn > ?
	`, cutoff, maxMonitorChecksPerMonitor).Error
}

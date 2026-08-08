package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Запись uptime: интервал между двумя проверками [prev, curr) относится к bucket с
// isUp *новой* проверки — это статус, который мы считаем действовавшим в «щели» до
// момента, когда узнали результат curr (retrospective attribution).

// UpdateUptimeStats относит интервал [предыдущая проверка, checkedAt) к минутным, часовым и дневным bucket.
// Для всего этого полуинтервала используется isUp *текущей* проверки: мы не знаем, что было
// между проверками, и приписываем щель статусу, который зафиксировали при приходе нового результата.
// db — дескриптор базы данных для персистентности.
// monitorID — монитор, чьи bucket обновляются.
// checkedAt — метка времени новой проверки.
// isUp — результат новой проверки, применяемый ко всему интервалу с момента предыдущей проверки.
func UpdateUptimeStats(db *gorm.DB, monitorID uint, checkedAt time.Time, isUp bool) error {
	var lastCheck MonitorCheck
	// Ищем последнюю проверку строго ДО текущей — интервал [prev, curr) ещё не записан.
	err := db.Where("monitor_url_id = ? AND checked_at < ?", monitorID, checkedAt).
		Order("checked_at desc").
		First(&lastCheck).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Первая проверка монитора: нечего агрегировать между двумя точками.
		return nil
	}
	if err != nil {
		return err
	}

	return ApplyUptimeInterval(db, monitorID, &lastCheck.CheckedAt, checkedAt, isUp)
}

// ApplyUptimeInterval относит полуинтервал [previousCheckedAt, checkedAt) к bucket uptime.
// Граница curr исключается: секунды до checkedAt идут под isUp новой проверки, а не под
// результат предыдущей — так согласованы UpdateUptimeStats и пересборка BackfillUptimeStats.
// db — дескриптор базы данных для персистентности.
// monitorID — монитор, чьи bucket обновляются.
// previousCheckedAt — время предыдущей проверки; nil означает первую проверку, bucket не меняются.
// checkedAt — метка времени новой проверки (правая граница, не включается).
// isUp — результат новой проверки для всего интервала [previousCheckedAt, checkedAt).
func ApplyUptimeInterval(db *gorm.DB, monitorID uint, previousCheckedAt *time.Time, checkedAt time.Time, isUp bool) error {
	if previousCheckedAt == nil {
		// Нет предыдущей точки — интервал пустой, bucket не трогаем.
		return nil
	}
	return addUptimeDuration(db, monitorID, *previousCheckedAt, checkedAt, isUp)
}

// BackfillUptimeStats перестраивает bucket uptime из существующей истории проверок мониторов.
// db — дескриптор базы данных для очистки и перезаписи агрегированной статистики.
func BackfillUptimeStats(db *gorm.DB) error {
	// Полная пересборка: сначала очищаем все три stat-таблицы (WHERE 1=1 — без условия GORM не DELETE ALL).
	if err := db.Where("1 = 1").Delete(&StatMinutely{}).Error; err != nil {
		return err
	}
	if err := db.Where("1 = 1").Delete(&StatHourly{}).Error; err != nil {
		return err
	}
	if err := db.Where("1 = 1").Delete(&StatDaily{}).Error; err != nil {
		return err
	}

	var monitorIDs []uint
	if err := db.Model(&MonitorURL{}).Pluck("id", &monitorIDs).Error; err != nil {
		return err
	}

	// Для каждого монитора проходим пару соседних проверок и накапливаем интервалы заново.
	for _, monitorID := range monitorIDs {
		var checks []MonitorCheck
		if err := db.Where("monitor_url_id = ?", monitorID).
			Order("checked_at asc").
			Find(&checks).Error; err != nil {
			return err
		}
		for i := 1; i < len(checks); i++ {
			prev := checks[i-1]
			curr := checks[i]
			// [prev, curr) относим к curr.IsUp: та же модель, что при живой записи — щель
			// до curr получает статус, который мы узнали только при curr.
			if err := addUptimeDuration(db, monitorID, prev.CheckedAt, curr.CheckedAt, curr.IsUp); err != nil {
				return err
			}
		}
	}

	return nil
}

// PruneUptimeStats удаляет bucket uptime старше окна хранения для каждой гранулярности.
// db — дескриптор базы данных для удалений.
func PruneUptimeStats(db *gorm.DB) error {
	now := time.Now()
	// У каждой гранулярности своё окно хранения — удаляем bucket старше cutoff.
	if err := db.Where("bucket_at < ?", now.Add(-minutelyStatRetention)).Delete(&StatMinutely{}).Error; err != nil {
		return err
	}
	if err := db.Where("bucket_at < ?", now.Add(-hourlyStatRetention)).Delete(&StatHourly{}).Error; err != nil {
		return err
	}
	if err := db.Where("bucket_at < ?", now.Add(-dailyStatRetention)).Delete(&StatDaily{}).Error; err != nil {
		return err
	}
	return nil
}

// addUptimeDuration разбивает один интервал [start, end) и записывает его во все три
// stat-таблицы (минутная, часовая, дневная) с одним и тем же isUp.
func addUptimeDuration(db *gorm.DB, monitorID uint, start, end time.Time, isUp bool) error {
	if !end.After(start) {
		// Нулевой или отрицательный интервал — нечего записывать.
		return nil
	}

	// Один интервал пишется во все три таблицы агрегации с одинаковым isUp.
	if err := addDurationToGranularity(db, monitorID, start, end, isUp, uptimeGranularityMinutely); err != nil {
		return err
	}
	if err := addDurationToGranularity(db, monitorID, start, end, isUp, uptimeGranularityHourly); err != nil {
		return err
	}
	return addDurationToGranularity(db, monitorID, start, end, isUp, uptimeGranularityDaily)
}

// addDurationToGranularity нарезает [start, end) по границам bucket одной гранулярности
// и для каждого куска вызывает upsertUptimeBucket.
func addDurationToGranularity(db *gorm.DB, monitorID uint, start, end time.Time, isUp bool, granularity uptimeGranularity) error {
	current := start
	// Идём по bucket-границам, пока не покроем весь полуинтервал [start, end).
	for current.Before(end) {
		bucketStart := truncateToBucket(current, granularity)
		bucketEnd := bucketStart.Add(bucketDuration(granularity))

		// Обрезаем сегмент пересечением [start, end) с текущим bucket [bucketStart, bucketEnd).
		segmentStart := current
		if segmentStart.Before(bucketStart) {
			segmentStart = bucketStart
		}
		segmentEnd := end
		if segmentEnd.After(bucketEnd) {
			segmentEnd = bucketEnd
		}

		seconds := int(segmentEnd.Sub(segmentStart).Seconds())
		if seconds > 0 {
			upSeconds := 0
			if isUp {
				upSeconds = seconds
			}
			if err := upsertUptimeBucket(db, monitorID, bucketStart, upSeconds, seconds, granularity); err != nil {
				return err
			}
		}

		// Следующий сегмент начинается с конца bucket (интервал может пересекать несколько bucket).
		current = bucketEnd
	}
	return nil
}

// upsertUptimeBucket вставляет строку bucket или при конфликте по (monitor_url_id, bucket_at)
// *прибавляет* секунды к существующим — накопление, а не перезапись.
func upsertUptimeBucket(db *gorm.DB, monitorID uint, bucketAt time.Time, upSeconds, totalSeconds int, granularity uptimeGranularity) error {
	switch granularity {
	case uptimeGranularityMinutely:
		// INSERT … ON CONFLICT DO UPDATE: секунды суммируются, а не перезаписываются.
		row := StatMinutely{
			MonitorURLID: monitorID,
			BucketAt:     bucketAt.UTC(),
			UpSeconds:    upSeconds,
			TotalSeconds: totalSeconds,
		}
		return db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "monitor_url_id"}, {Name: "bucket_at"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"up_seconds":    gorm.Expr("stat_minutely.up_seconds + ?", upSeconds),
				"total_seconds": gorm.Expr("stat_minutely.total_seconds + ?", totalSeconds),
			}),
		}).Create(&row).Error
	case uptimeGranularityHourly:
		row := StatHourly{
			MonitorURLID: monitorID,
			BucketAt:     bucketAt.UTC(),
			UpSeconds:    upSeconds,
			TotalSeconds: totalSeconds,
		}
		return db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "monitor_url_id"}, {Name: "bucket_at"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"up_seconds":    gorm.Expr("stat_hourly.up_seconds + ?", upSeconds),
				"total_seconds": gorm.Expr("stat_hourly.total_seconds + ?", totalSeconds),
			}),
		}).Create(&row).Error
	case uptimeGranularityDaily:
		// Тот же upsert-паттерн для дневных bucket.
		row := StatDaily{
			MonitorURLID: monitorID,
			BucketAt:     bucketAt.UTC(),
			UpSeconds:    upSeconds,
			TotalSeconds: totalSeconds,
		}
		return db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "monitor_url_id"}, {Name: "bucket_at"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"up_seconds":    gorm.Expr("stat_daily.up_seconds + ?", upSeconds),
				"total_seconds": gorm.Expr("stat_daily.total_seconds + ?", totalSeconds),
			}),
		}).Create(&row).Error
	default:
		return fmt.Errorf("unknown uptime granularity: %s", granularity)
	}
}

// truncateToBucket возвращает начало bucket в UTC: минута — до секунд, час — до минут, день — до часов.
func truncateToBucket(t time.Time, granularity uptimeGranularity) time.Time {
	utc := t.UTC()
	switch granularity {
	case uptimeGranularityMinutely:
		// Обнуляем секунды и наносекунды — начало минуты в UTC.
		return time.Date(utc.Year(), utc.Month(), utc.Day(), utc.Hour(), utc.Minute(), 0, 0, time.UTC)
	case uptimeGranularityHourly:
		// Обнуляем минуты и ниже — начало часа в UTC.
		return time.Date(utc.Year(), utc.Month(), utc.Day(), utc.Hour(), 0, 0, 0, time.UTC)
	case uptimeGranularityDaily:
		// Обнуляем часы и ниже — начало календарного дня в UTC.
		return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	default:
		return utc
	}
}

// bucketDuration — длина одного bucket для гранулярности (1 мин / 1 ч / 24 ч).
func bucketDuration(granularity uptimeGranularity) time.Duration {
	switch granularity {
	case uptimeGranularityMinutely:
		return time.Minute
	case uptimeGranularityHourly:
		return time.Hour
	case uptimeGranularityDaily:
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

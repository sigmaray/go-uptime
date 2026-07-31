package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpdateUptimeStats attributes the interval since the previous check into minutely, hourly, and daily buckets.
// db is the database handle used for persistence.
// monitorID is the monitor whose buckets are updated.
// checkedAt is the timestamp of the new check.
// isUp is the result of the new check used for the interval since the previous check.
func UpdateUptimeStats(db *gorm.DB, monitorID uint, checkedAt time.Time, isUp bool) error {
	var lastCheck MonitorCheck
	err := db.Where("monitor_url_id = ? AND checked_at < ?", monitorID, checkedAt).
		Order("checked_at desc").
		First(&lastCheck).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	return ApplyUptimeInterval(db, monitorID, &lastCheck.CheckedAt, checkedAt, isUp)
}

// ApplyUptimeInterval attributes [previousCheckedAt, checkedAt) into uptime buckets when a prior check exists.
// db is the database handle used for persistence.
// monitorID is the monitor whose buckets are updated.
// previousCheckedAt is the prior check time; nil means this is the first check and no buckets change.
// checkedAt is the timestamp of the new check.
// isUp is the new check result applied to the interval since the previous check.
func ApplyUptimeInterval(db *gorm.DB, monitorID uint, previousCheckedAt *time.Time, checkedAt time.Time, isUp bool) error {
	if previousCheckedAt == nil {
		return nil
	}
	return addUptimeDuration(db, monitorID, *previousCheckedAt, checkedAt, isUp)
}

// BackfillUptimeStats rebuilds uptime buckets from existing monitor check history.
// db is the database handle used to clear and rewrite aggregated stats.
func BackfillUptimeStats(db *gorm.DB) error {
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
			if err := addUptimeDuration(db, monitorID, prev.CheckedAt, curr.CheckedAt, curr.IsUp); err != nil {
				return err
			}
		}
	}

	return nil
}

// PruneUptimeStats removes uptime buckets older than each granularity retention window.
// db is the database handle used for deletions.
func PruneUptimeStats(db *gorm.DB) error {
	now := time.Now()
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

func addUptimeDuration(db *gorm.DB, monitorID uint, start, end time.Time, isUp bool) error {
	if !end.After(start) {
		return nil
	}

	if err := addDurationToGranularity(db, monitorID, start, end, isUp, uptimeGranularityMinutely); err != nil {
		return err
	}
	if err := addDurationToGranularity(db, monitorID, start, end, isUp, uptimeGranularityHourly); err != nil {
		return err
	}
	return addDurationToGranularity(db, monitorID, start, end, isUp, uptimeGranularityDaily)
}

func addDurationToGranularity(db *gorm.DB, monitorID uint, start, end time.Time, isUp bool, granularity uptimeGranularity) error {
	current := start
	for current.Before(end) {
		bucketStart := truncateToBucket(current, granularity)
		bucketEnd := bucketStart.Add(bucketDuration(granularity))

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

		current = bucketEnd
	}
	return nil
}

func upsertUptimeBucket(db *gorm.DB, monitorID uint, bucketAt time.Time, upSeconds, totalSeconds int, granularity uptimeGranularity) error {
	switch granularity {
	case uptimeGranularityMinutely:
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

func truncateToBucket(t time.Time, granularity uptimeGranularity) time.Time {
	utc := t.UTC()
	switch granularity {
	case uptimeGranularityMinutely:
		return time.Date(utc.Year(), utc.Month(), utc.Day(), utc.Hour(), utc.Minute(), 0, 0, time.UTC)
	case uptimeGranularityHourly:
		return time.Date(utc.Year(), utc.Month(), utc.Day(), utc.Hour(), 0, 0, 0, time.UTC)
	case uptimeGranularityDaily:
		return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	default:
		return utc
	}
}

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

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

// UptimeInterval describes one monitor's final active interval before a lifecycle change.
type UptimeInterval struct {
	// MonitorURLID identifies the monitor whose uptime statistics are updated.
	MonitorURLID uint
	// StartedAt is the timestamp of the monitor's most recent check.
	StartedAt time.Time
	// EndedAt is the timestamp at which the monitor is paused.
	EndedAt time.Time
	// IsUp is the last observed monitor state used for this interval.
	IsUp bool
}

// ApplyUptimeIntervals adds many monitor intervals to uptime buckets using batched inserts.
// db is the transaction handle used to persist all generated bucket updates.
// intervals contains the last observed active interval for each monitor being paused.
func ApplyUptimeIntervals(db *gorm.DB, intervals []UptimeInterval) error {
	const batchSize = 500

	minutely := make([]StatMinutely, 0, batchSize)
	hourly := make([]StatHourly, 0, batchSize)
	daily := make([]StatDaily, 0, batchSize)
	flush := func() error {
		if len(minutely) > 0 {
			if err := upsertMinutelyBatch(db, minutely); err != nil {
				return err
			}
			minutely = minutely[:0]
		}
		if len(hourly) > 0 {
			if err := upsertHourlyBatch(db, hourly); err != nil {
				return err
			}
			hourly = hourly[:0]
		}
		if len(daily) > 0 {
			if err := upsertDailyBatch(db, daily); err != nil {
				return err
			}
			daily = daily[:0]
		}
		return nil
	}

	for _, interval := range intervals {
		if err := collectDurationBuckets(interval, uptimeGranularityMinutely, func(bucketAt time.Time, seconds int) error {
			upSeconds := 0
			if interval.IsUp {
				upSeconds = seconds
			}
			minutely = append(minutely, StatMinutely{MonitorURLID: interval.MonitorURLID, BucketAt: bucketAt, UpSeconds: upSeconds, TotalSeconds: seconds})
			if len(minutely) >= batchSize {
				return flush()
			}
			return nil
		}); err != nil {
			return err
		}
		if err := collectDurationBuckets(interval, uptimeGranularityHourly, func(bucketAt time.Time, seconds int) error {
			upSeconds := 0
			if interval.IsUp {
				upSeconds = seconds
			}
			hourly = append(hourly, StatHourly{MonitorURLID: interval.MonitorURLID, BucketAt: bucketAt, UpSeconds: upSeconds, TotalSeconds: seconds})
			if len(hourly) >= batchSize {
				return flush()
			}
			return nil
		}); err != nil {
			return err
		}
		if err := collectDurationBuckets(interval, uptimeGranularityDaily, func(bucketAt time.Time, seconds int) error {
			upSeconds := 0
			if interval.IsUp {
				upSeconds = seconds
			}
			daily = append(daily, StatDaily{MonitorURLID: interval.MonitorURLID, BucketAt: bucketAt, UpSeconds: upSeconds, TotalSeconds: seconds})
			if len(daily) >= batchSize {
				return flush()
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return flush()
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
	return collectDurationBuckets(UptimeInterval{MonitorURLID: monitorID, StartedAt: start, EndedAt: end, IsUp: isUp}, granularity, func(bucketAt time.Time, seconds int) error {
		upSeconds := 0
		if isUp {
			upSeconds = seconds
		}
		return upsertUptimeBucket(db, monitorID, bucketAt, upSeconds, seconds, granularity)
	})
}

// collectDurationBuckets sends each nonempty bucket segment of interval to add.
// interval is the monitor time range being split into uptime buckets.
// granularity selects minute, hour, or day boundaries.
// add persists or collects a bucket and its duration in seconds.
func collectDurationBuckets(interval UptimeInterval, granularity uptimeGranularity, add func(time.Time, int) error) error {
	current := interval.StartedAt
	for current.Before(interval.EndedAt) {
		bucketStart := truncateToBucket(current, granularity)
		bucketEnd := bucketStart.Add(bucketDuration(granularity))
		segmentEnd := interval.EndedAt
		if segmentEnd.After(bucketEnd) {
			segmentEnd = bucketEnd
		}

		seconds := int(segmentEnd.Sub(current).Seconds())
		if seconds > 0 {
			if err := add(bucketStart, seconds); err != nil {
				return err
			}
		}
		current = bucketEnd
	}
	return nil
}

// upsertMinutelyBatch adds minute bucket deltas in one or more database statements.
// db is the active transaction handle.
// rows contains minute bucket deltas to add.
func upsertMinutelyBatch(db *gorm.DB, rows []StatMinutely) error {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "monitor_url_id"}, {Name: "bucket_at"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"up_seconds":    gorm.Expr("stat_minutely.up_seconds + EXCLUDED.up_seconds"),
			"total_seconds": gorm.Expr("stat_minutely.total_seconds + EXCLUDED.total_seconds"),
		}),
	}).CreateInBatches(&rows, 500).Error
}

// upsertHourlyBatch adds hour bucket deltas in one or more database statements.
// db is the active transaction handle.
// rows contains hour bucket deltas to add.
func upsertHourlyBatch(db *gorm.DB, rows []StatHourly) error {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "monitor_url_id"}, {Name: "bucket_at"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"up_seconds":    gorm.Expr("stat_hourly.up_seconds + EXCLUDED.up_seconds"),
			"total_seconds": gorm.Expr("stat_hourly.total_seconds + EXCLUDED.total_seconds"),
		}),
	}).CreateInBatches(&rows, 500).Error
}

// upsertDailyBatch adds day bucket deltas in one or more database statements.
// db is the active transaction handle.
// rows contains day bucket deltas to add.
func upsertDailyBatch(db *gorm.DB, rows []StatDaily) error {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "monitor_url_id"}, {Name: "bucket_at"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"up_seconds":    gorm.Expr("stat_daily.up_seconds + EXCLUDED.up_seconds"),
			"total_seconds": gorm.Expr("stat_daily.total_seconds + EXCLUDED.total_seconds"),
		}),
	}).CreateInBatches(&rows, 500).Error
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

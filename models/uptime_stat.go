package models

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// minutelyStatRetention is how long per-minute uptime buckets are kept.
	minutelyStatRetention = 24 * time.Hour
	// hourlyStatRetention is how long per-hour uptime buckets are kept.
	hourlyStatRetention = 30 * 24 * time.Hour
	// dailyStatRetention is how long per-day uptime buckets are kept.
	dailyStatRetention = 365 * 24 * time.Hour
	// uptimeHistoryMinutes is how many one-minute slots the monitor list chart shows.
	uptimeHistoryMinutes = 30
)

// UptimeBarState classifies one minute slot in the 30-minute uptime chart.
type UptimeBarState string

const (
	// UptimeBarNoData marks a minute before the monitor existed or without check data.
	UptimeBarNoData UptimeBarState = "nodata"
	// UptimeBarUp marks a minute where the monitor was fully up.
	UptimeBarUp UptimeBarState = "up"
	// UptimeBarDown marks a minute where the monitor was fully down.
	UptimeBarDown UptimeBarState = "down"
	// UptimeBarMixed marks a minute with partial uptime within the bucket.
	UptimeBarMixed UptimeBarState = "mixed"
)

// UptimeHistoryBar is one minute in the recent uptime strip shown on monitor pages.
type UptimeHistoryBar struct {
	BucketAt time.Time
	State    UptimeBarState
}

// Title returns a tooltip describing the minute bucket state.
func (b UptimeHistoryBar) Title() string {
	switch b.State {
	case UptimeBarUp:
		return b.BucketAt.Format("15:04") + " — Up"
	case UptimeBarDown:
		return b.BucketAt.Format("15:04") + " — Down"
	case UptimeBarMixed:
		return b.BucketAt.Format("15:04") + " — Mixed"
	default:
		return b.BucketAt.Format("15:04") + " — No data"
	}
}

// StatMinutely stores uptime seconds aggregated into one-minute buckets (24h window).
type StatMinutely struct {
	MonitorURLID uint      `gorm:"primaryKey"`
	BucketAt     time.Time `gorm:"primaryKey"`
	UpSeconds    int       `gorm:"not null;default:0"`
	TotalSeconds int       `gorm:"not null;default:0"`
}

// TableName returns the database table name for StatMinutely.
func (StatMinutely) TableName() string { return "stat_minutely" }

// StatHourly stores uptime seconds aggregated into one-hour buckets (30d window).
type StatHourly struct {
	MonitorURLID uint      `gorm:"primaryKey"`
	BucketAt     time.Time `gorm:"primaryKey"`
	UpSeconds    int       `gorm:"not null;default:0"`
	TotalSeconds int       `gorm:"not null;default:0"`
}

// TableName returns the database table name for StatHourly.
func (StatHourly) TableName() string { return "stat_hourly" }

// StatDaily stores uptime seconds aggregated into one-day buckets (365d window).
type StatDaily struct {
	MonitorURLID uint      `gorm:"primaryKey"`
	BucketAt     time.Time `gorm:"primaryKey"`
	UpSeconds    int       `gorm:"not null;default:0"`
	TotalSeconds int       `gorm:"not null;default:0"`
}

// TableName returns the database table name for StatDaily.
func (StatDaily) TableName() string { return "stat_daily" }

// UptimeSummary holds raw uptime seconds for one reporting window.
type UptimeSummary struct {
	UpSeconds    int64
	TotalSeconds int64
}

// HasData reports whether any uptime duration was recorded for the window.
func (s UptimeSummary) HasData() bool {
	return s.TotalSeconds > 0
}

// Percent returns the uptime percentage rounded to two decimal places, or -1 when no data exists.
func (s UptimeSummary) Percent() float64 {
	if s.TotalSeconds == 0 {
		return -1
	}
	pct := float64(s.UpSeconds) / float64(s.TotalSeconds) * 100
	return math.Round(pct*100) / 100
}

// MonitorUptime groups uptime summaries for the standard reporting periods.
type MonitorUptime struct {
	Hour1   UptimeSummary
	Hours24 UptimeSummary
	Days30  UptimeSummary
	Year1   UptimeSummary
}

type uptimeGranularity string

const (
	uptimeGranularityMinutely uptimeGranularity = "minutely"
	uptimeGranularityHourly   uptimeGranularity = "hourly"
	uptimeGranularityDaily    uptimeGranularity = "daily"
)

// FormatUptimePercent renders an uptime percentage string or a dash when data is missing.
func FormatUptimePercent(summary UptimeSummary) string {
	if !summary.HasData() {
		return "—"
	}
	return fmt.Sprintf("%.2f%%", summary.Percent())
}

// UpdateUptimeStats attributes the interval since the previous check into minutely, hourly, and daily buckets.
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

	return addUptimeDuration(db, monitorID, lastCheck.CheckedAt, checkedAt, isUp)
}

// BackfillUptimeStats rebuilds uptime buckets from existing monitor check history.
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

// LoadMonitorUptime returns uptime summaries for one monitor across 1h, 24h, 30d, and 365d windows.
// createdAt limits each window to the time after the monitor was created; younger monitors
// return empty summaries for periods they have not existed long enough to cover.
func LoadMonitorUptime(db *gorm.DB, monitorID uint, createdAt, now time.Time) (MonitorUptime, error) {
	hour1, err := loadUptimeSummary(db, monitorID, createdAt, now, time.Hour, uptimeGranularityMinutely)
	if err != nil {
		return MonitorUptime{}, err
	}
	hours24, err := loadUptimeSummary(db, monitorID, createdAt, now, minutelyStatRetention, uptimeGranularityMinutely)
	if err != nil {
		return MonitorUptime{}, err
	}
	days30, err := loadUptimeSummary(db, monitorID, createdAt, now, hourlyStatRetention, uptimeGranularityHourly)
	if err != nil {
		return MonitorUptime{}, err
	}
	year1, err := loadUptimeSummary(db, monitorID, createdAt, now, dailyStatRetention, uptimeGranularityDaily)
	if err != nil {
		return MonitorUptime{}, err
	}

	return MonitorUptime{
		Hour1:   hour1,
		Hours24: hours24,
		Days30:  days30,
		Year1:   year1,
	}, nil
}

// LoadMonitorUptimes returns uptime summaries for many monitors in three queries.
// createdAtByID supplies each monitor creation time used to clip reporting windows.
func LoadMonitorUptimes(db *gorm.DB, monitorIDs []uint, createdAtByID map[uint]time.Time, now time.Time) (map[uint]MonitorUptime, error) {
	result := make(map[uint]MonitorUptime, len(monitorIDs))
	if len(monitorIDs) == 0 {
		return result, nil
	}

	hour1, err := sumUptimeBucketsForMonitors(db, monitorIDs, createdAtByID, now, time.Hour, uptimeGranularityMinutely)
	if err != nil {
		return nil, err
	}
	hours24, err := sumUptimeBucketsForMonitors(db, monitorIDs, createdAtByID, now, minutelyStatRetention, uptimeGranularityMinutely)
	if err != nil {
		return nil, err
	}
	days30, err := sumUptimeBucketsForMonitors(db, monitorIDs, createdAtByID, now, hourlyStatRetention, uptimeGranularityHourly)
	if err != nil {
		return nil, err
	}
	year1, err := sumUptimeBucketsForMonitors(db, monitorIDs, createdAtByID, now, dailyStatRetention, uptimeGranularityDaily)
	if err != nil {
		return nil, err
	}

	for _, id := range monitorIDs {
		result[id] = MonitorUptime{
			Hour1:   hour1[id],
			Hours24: hours24[id],
			Days30:  days30[id],
			Year1:   year1[id],
		}
	}
	return result, nil
}

// BuildUptimeHistoryBars builds the fixed 30-minute uptime strip for one monitor.
// Minutes before createdAt or without bucket data are marked as no-data.
func BuildUptimeHistoryBars(db *gorm.DB, monitorID uint, createdAt, now time.Time) ([]UptimeHistoryBar, error) {
	bucketsByMonitor, err := LoadUptimeHistoryBarsForMonitors(db, []uint{monitorID}, map[uint]time.Time{monitorID: createdAt}, now)
	if err != nil {
		return nil, err
	}
	return bucketsByMonitor[monitorID], nil
}

// LoadUptimeHistoryBarsForMonitors builds 30-minute uptime strips for many monitors in one query.
func LoadUptimeHistoryBarsForMonitors(db *gorm.DB, monitorIDs []uint, createdAtByID map[uint]time.Time, now time.Time) (map[uint][]UptimeHistoryBar, error) {
	result := make(map[uint][]UptimeHistoryBar, len(monitorIDs))
	if len(monitorIDs) == 0 {
		return result, nil
	}

	windowEnd := now.Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(uptimeHistoryMinutes-1) * time.Minute)

	var rows []StatMinutely
	if err := db.Where("monitor_url_id IN ? AND bucket_at >= ? AND bucket_at <= ?", monitorIDs, windowStart.UTC(), windowEnd.UTC()).
		Order("bucket_at asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	bucketsByMonitor := make(map[uint]map[time.Time]StatMinutely, len(monitorIDs))
	for _, row := range rows {
		if bucketsByMonitor[row.MonitorURLID] == nil {
			bucketsByMonitor[row.MonitorURLID] = make(map[time.Time]StatMinutely)
		}
		bucketsByMonitor[row.MonitorURLID][row.BucketAt.UTC()] = row
	}

	for _, monitorID := range monitorIDs {
		createdAt := createdAtByID[monitorID]
		createdMinute := createdAt.Truncate(time.Minute)
		buckets := bucketsByMonitor[monitorID]

		bars := make([]UptimeHistoryBar, 0, uptimeHistoryMinutes)
		for i := 0; i < uptimeHistoryMinutes; i++ {
			bucketAt := windowStart.Add(time.Duration(i) * time.Minute)
			if bucketAt.Before(createdMinute) {
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarNoData})
				continue
			}

			bucket, ok := buckets[bucketAt.UTC()]
			if !ok || bucket.TotalSeconds == 0 {
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarNoData})
				continue
			}

			switch {
			case bucket.UpSeconds >= bucket.TotalSeconds:
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarUp})
			case bucket.UpSeconds == 0:
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarDown})
			default:
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarMixed})
			}
		}
		result[monitorID] = bars
	}

	return result, nil
}

func loadUptimeSummary(db *gorm.DB, monitorID uint, createdAt, now time.Time, period time.Duration, granularity uptimeGranularity) (UptimeSummary, error) {
	if !uptimePeriodEligible(createdAt, now, period) {
		return UptimeSummary{}, nil
	}
	since := effectiveSince(createdAt, now.Add(-period))
	return sumUptimeBuckets(db, monitorID, since, granularity)
}

// uptimePeriodEligible reports whether the monitor has existed for the full reporting period.
func uptimePeriodEligible(createdAt, now time.Time, period time.Duration) bool {
	return !createdAt.After(now.Add(-period))
}

// effectiveSince returns the later of the reporting window start and monitor creation time.
func effectiveSince(createdAt, since time.Time) time.Time {
	if createdAt.After(since) {
		return createdAt
	}
	return since
}

// PruneUptimeStats removes uptime buckets older than each granularity retention window.
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

func sumUptimeBuckets(db *gorm.DB, monitorID uint, since time.Time, granularity uptimeGranularity) (UptimeSummary, error) {
	var result uptimeSummaryRow

	switch granularity {
	case uptimeGranularityMinutely:
		err := db.Model(&StatMinutely{}).
			Select("COALESCE(SUM(up_seconds), 0) AS up_seconds, COALESCE(SUM(total_seconds), 0) AS total_seconds").
			Where("monitor_url_id = ? AND bucket_at >= ?", monitorID, since.UTC()).
			Scan(&result).Error
		return result.summary(), err
	case uptimeGranularityHourly:
		err := db.Model(&StatHourly{}).
			Select("COALESCE(SUM(up_seconds), 0) AS up_seconds, COALESCE(SUM(total_seconds), 0) AS total_seconds").
			Where("monitor_url_id = ? AND bucket_at >= ?", monitorID, since.UTC()).
			Scan(&result).Error
		return result.summary(), err
	case uptimeGranularityDaily:
		err := db.Model(&StatDaily{}).
			Select("COALESCE(SUM(up_seconds), 0) AS up_seconds, COALESCE(SUM(total_seconds), 0) AS total_seconds").
			Where("monitor_url_id = ? AND bucket_at >= ?", monitorID, since.UTC()).
			Scan(&result).Error
		return result.summary(), err
	default:
		return UptimeSummary{}, fmt.Errorf("unknown uptime granularity: %s", granularity)
	}
}

type uptimeSummaryRow struct {
	UpSeconds    int64
	TotalSeconds int64
}

func (r uptimeSummaryRow) summary() UptimeSummary {
	return UptimeSummary(r)
}

func sumUptimeBucketsForMonitors(db *gorm.DB, monitorIDs []uint, createdAtByID map[uint]time.Time, now time.Time, period time.Duration, granularity uptimeGranularity) (map[uint]UptimeSummary, error) {
	result := make(map[uint]UptimeSummary, len(monitorIDs))
	eligibleIDs := make([]uint, 0, len(monitorIDs))
	sinceByID := make(map[uint]time.Time, len(monitorIDs))
	windowSince := now.Add(-period)

	for _, id := range monitorIDs {
		createdAt := createdAtByID[id]
		if !uptimePeriodEligible(createdAt, now, period) {
			result[id] = UptimeSummary{}
			continue
		}
		eligibleIDs = append(eligibleIDs, id)
		sinceByID[id] = effectiveSince(createdAt, windowSince)
	}

	if len(eligibleIDs) == 0 {
		return result, nil
	}

	type row struct {
		MonitorURLID uint
		uptimeSummaryRow
	}

	var rows []row
	tableName := tableNameForGranularity(granularity)
	for _, id := range eligibleIDs {
		var summary uptimeSummaryRow
		err := db.Table(tableName).
			Select("COALESCE(SUM(up_seconds), 0) AS up_seconds, COALESCE(SUM(total_seconds), 0) AS total_seconds").
			Where("monitor_url_id = ? AND bucket_at >= ?", id, sinceByID[id].UTC()).
			Scan(&summary).Error
		if err != nil {
			return nil, err
		}
		rows = append(rows, row{MonitorURLID: id, uptimeSummaryRow: summary})
	}

	for _, id := range eligibleIDs {
		result[id] = UptimeSummary{}
	}
	for _, row := range rows {
		result[row.MonitorURLID] = row.summary()
	}
	return result, nil
}

func tableNameForGranularity(granularity uptimeGranularity) string {
	switch granularity {
	case uptimeGranularityMinutely:
		return "stat_minutely"
	case uptimeGranularityHourly:
		return "stat_hourly"
	case uptimeGranularityDaily:
		return "stat_daily"
	default:
		return ""
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

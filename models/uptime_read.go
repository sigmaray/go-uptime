package models

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// uptimeMonitorSummaryRow is one aggregated uptime result for a monitor.
type uptimeMonitorSummaryRow struct {
	// MonitorURLID is the monitor_urls.id whose buckets were summed.
	MonitorURLID uint
	uptimeSummaryRow
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

// LoadMonitorUptimes returns uptime summaries for many monitors without per-monitor queries.
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

	tableName := tableNameForGranularity(granularity)
	if tableName == "" {
		return nil, fmt.Errorf("unknown uptime granularity: %s", granularity)
	}

	rows, err := sumUptimeBucketsForEligibleMonitors(db, tableName, eligibleIDs, sinceByID)
	if err != nil {
		return nil, err
	}

	for _, id := range eligibleIDs {
		result[id] = UptimeSummary{}
	}
	for _, row := range rows {
		result[row.MonitorURLID] = row.summary()
	}
	return result, nil
}

// sumUptimeBucketsForEligibleMonitors aggregates uptime buckets for many monitors in one SQL query.
// db is the database handle; tableName is a trusted stat table name; monitorIDs are eligible monitors;
// sinceByID contains each monitor's lower bucket boundary.
func sumUptimeBucketsForEligibleMonitors(db *gorm.DB, tableName string, monitorIDs []uint, sinceByID map[uint]time.Time) ([]uptimeMonitorSummaryRow, error) {
	valuesSQL, args := uptimeBucketRequestValues(monitorIDs, sinceByID)
	query := fmt.Sprintf(`
		WITH requested(monitor_url_id, since_at) AS (
			VALUES %s
		)
		SELECT
			requested.monitor_url_id,
			COALESCE(SUM(stats.up_seconds), 0) AS up_seconds,
			COALESCE(SUM(stats.total_seconds), 0) AS total_seconds
		FROM requested
		LEFT JOIN %s AS stats
			ON stats.monitor_url_id = requested.monitor_url_id
			AND stats.bucket_at >= requested.since_at
		GROUP BY requested.monitor_url_id
	`, valuesSQL, tableName)

	var rows []uptimeMonitorSummaryRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// uptimeBucketRequestValues builds the VALUES list for per-monitor uptime windows.
// monitorIDs are emitted in order; sinceByID supplies the lower bucket boundary for each id.
func uptimeBucketRequestValues(monitorIDs []uint, sinceByID map[uint]time.Time) (string, []interface{}) {
	var b strings.Builder
	args := make([]interface{}, 0, len(monitorIDs)*2)
	for i, id := range monitorIDs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(?::bigint, ?::timestamptz)")
		args = append(args, id, sinceByID[id].UTC())
	}
	return b.String(), args
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

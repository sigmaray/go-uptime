package models

import (
	"math"
	"time"
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

type uptimeSummaryRow struct {
	UpSeconds    int64
	TotalSeconds int64
}

func (r uptimeSummaryRow) summary() UptimeSummary {
	return UptimeSummary(r)
}

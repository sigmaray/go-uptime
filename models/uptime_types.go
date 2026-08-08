package models

import (
	"math"
	"time"
)

const (
	// minutelyStatRetention — как долго хранятся минутные bucket uptime.
	minutelyStatRetention = 24 * time.Hour
	// hourlyStatRetention — как долго хранятся часовые bucket uptime.
	hourlyStatRetention = 30 * 24 * time.Hour
	// dailyStatRetention — как долго хранятся дневные bucket uptime.
	dailyStatRetention = 365 * 24 * time.Hour
	// uptimeHistoryMinutes — сколько одноминутных слотов показывает график списка мониторов.
	uptimeHistoryMinutes = 30
)

// UptimeBarState классифицирует один минутный слот на 30-минутном графике uptime.
type UptimeBarState string

const (
	// UptimeBarNoData помечает минуту до создания монитора или без данных проверок.
	UptimeBarNoData UptimeBarState = "nodata"
	// UptimeBarUp помечает минуту, когда монитор был полностью доступен.
	UptimeBarUp UptimeBarState = "up"
	// UptimeBarDown помечает минуту, когда монитор был полностью недоступен.
	UptimeBarDown UptimeBarState = "down"
	// UptimeBarMixed помечает минуту с частичным uptime внутри bucket.
	UptimeBarMixed UptimeBarState = "mixed"
)

// UptimeHistoryBar — одна минута на недавней полосе uptime, показываемой на страницах мониторов.
type UptimeHistoryBar struct {
	BucketAt time.Time
	State    UptimeBarState
}

// Title возвращает подсказку, описывающую состояние минутного bucket.
func (b UptimeHistoryBar) Title() string {
	// Текст подсказки для HTML title/minibar — время минуты + классификация состояния.
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

// StatMinutely хранит секунды uptime, агрегированные в одноминутные bucket (окно 24 ч).
type StatMinutely struct {
	MonitorURLID uint      `gorm:"primaryKey"`
	BucketAt     time.Time `gorm:"primaryKey"`
	UpSeconds    int       `gorm:"not null;default:0"`
	TotalSeconds int       `gorm:"not null;default:0"`
}

// TableName возвращает имя таблицы базы данных для StatMinutely.
func (StatMinutely) TableName() string { return "stat_minutely" }

// StatHourly хранит секунды uptime, агрегированные в часовые bucket (окно 30 д).
type StatHourly struct {
	MonitorURLID uint      `gorm:"primaryKey"`
	BucketAt     time.Time `gorm:"primaryKey"`
	UpSeconds    int       `gorm:"not null;default:0"`
	TotalSeconds int       `gorm:"not null;default:0"`
}

// TableName возвращает имя таблицы базы данных для StatHourly.
func (StatHourly) TableName() string { return "stat_hourly" }

// StatDaily хранит секунды uptime, агрегированные в дневные bucket (окно 365 д).
type StatDaily struct {
	MonitorURLID uint      `gorm:"primaryKey"`
	BucketAt     time.Time `gorm:"primaryKey"`
	UpSeconds    int       `gorm:"not null;default:0"`
	TotalSeconds int       `gorm:"not null;default:0"`
}

// TableName возвращает имя таблицы базы данных для StatDaily.
func (StatDaily) TableName() string { return "stat_daily" }

// UptimeSummary хранит сырые секунды uptime для одного отчётного окна.
type UptimeSummary struct {
	UpSeconds    int64
	TotalSeconds int64
}

// HasData сообщает, была ли записана какая-либо длительность uptime для окна.
func (s UptimeSummary) HasData() bool {
	return s.TotalSeconds > 0
}

// Percent возвращает процент uptime, округлённый до двух знаков после запятой, или -1, если данных нет.
func (s UptimeSummary) Percent() float64 {
	if s.TotalSeconds == 0 {
		// -1 — сигнал шаблону «процент недоступен», а не 0%.
		return -1
	}
	pct := float64(s.UpSeconds) / float64(s.TotalSeconds) * 100
	// Округление до двух знаков после запятой для UI.
	return math.Round(pct*100) / 100
}

// MonitorUptime группирует сводки uptime для стандартных отчётных периодов.
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

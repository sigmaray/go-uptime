package models

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// uptimeMonitorSummaryRow — один агрегированный результат uptime для монитора.
// Поля плоские (не встроенные), чтобы GORM Raw().Scan корректно сопоставлял имена SQL-колонок.
type uptimeMonitorSummaryRow struct {
	// MonitorURLID — monitor_urls.id, чьи bucket были просуммированы.
	MonitorURLID uint
	// UpSeconds — суммированные секунды uptime по совпавшим bucket.
	UpSeconds int64
	// TotalSeconds — суммированные наблюдаемые секунды по совпавшим bucket.
	TotalSeconds int64
}

// summary преобразует агрегированную SQL-строку в значение UptimeSummary.
func (r uptimeMonitorSummaryRow) summary() UptimeSummary {
	return UptimeSummary{UpSeconds: r.UpSeconds, TotalSeconds: r.TotalSeconds}
}

// LoadMonitorUptime возвращает сводки uptime для одного монитора за окна 1 ч, 24 ч, 30 д и 365 д.
// Гранулярность читается из той таблицы, где ещё есть данные за окно: 1 ч и 24 ч — minutely
// (retention 24 ч), 30 д — hourly (retention 30 д), 365 д — daily (retention 365 д).
// createdAt ограничивает каждое окно временем после создания монитора; более молодые мониторы
// возвращают пустые сводки для периодов, которых они ещё не существовали достаточно долго.
func LoadMonitorUptime(db *gorm.DB, monitorID uint, createdAt, now time.Time) (MonitorUptime, error) {
	// Четыре окна — четыре вызова с разной гранулярностью stat-таблиц.
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

// LoadMonitorUptimes возвращает сводки uptime для многих мониторов без отдельных запросов на каждый.
// createdAtByID содержит время создания каждого монитора, используемое для обрезки отчётных окон.
func LoadMonitorUptimes(db *gorm.DB, monitorIDs []uint, createdAtByID map[uint]time.Time, now time.Time) (map[uint]MonitorUptime, error) {
	result := make(map[uint]MonitorUptime, len(monitorIDs))
	if len(monitorIDs) == 0 {
		return result, nil
	}

	// Пакетная агрегация: по одному SQL на каждое отчётное окно вместо N×4 запросов.
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

	// Собираем map[id]MonitorUptime из четырёх map[id]UptimeSummary.
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

// BuildUptimeHistoryBars строит фиксированную 30-минутную полосу uptime для одного монитора.
// Минуты до createdAt или без данных bucket помечаются как «нет данных».
func BuildUptimeHistoryBars(db *gorm.DB, monitorID uint, createdAt, now time.Time) ([]UptimeHistoryBar, error) {
	bucketsByMonitor, err := LoadUptimeHistoryBarsForMonitors(db, []uint{monitorID}, map[uint]time.Time{monitorID: createdAt}, now)
	if err != nil {
		return nil, err
	}
	return bucketsByMonitor[monitorID], nil
}

// LoadUptimeHistoryBarsForMonitors строит 30-минутные полосы uptime для многих мониторов одним запросом.
func LoadUptimeHistoryBarsForMonitors(db *gorm.DB, monitorIDs []uint, createdAtByID map[uint]time.Time, now time.Time) (map[uint][]UptimeHistoryBar, error) {
	result := make(map[uint][]UptimeHistoryBar, len(monitorIDs))
	if len(monitorIDs) == 0 {
		return result, nil
	}

	// Окно графика: последние uptimeHistoryMinutes минут, включая текущую.
	windowEnd := now.Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(uptimeHistoryMinutes-1) * time.Minute)

	// Один запрос минутных bucket для всех мониторов на странице списка.
	var rows []StatMinutely
	if err := db.Where("monitor_url_id IN ? AND bucket_at >= ? AND bucket_at <= ?", monitorIDs, windowStart.UTC(), windowEnd.UTC()).
		Order("bucket_at asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	// Индексируем строки stat_minutely по monitor_id → bucket_at для O(1) lookup при сборке полосы.
	bucketsByMonitor := make(map[uint]map[time.Time]StatMinutely, len(monitorIDs))
	for _, row := range rows {
		if bucketsByMonitor[row.MonitorURLID] == nil {
			bucketsByMonitor[row.MonitorURLID] = make(map[time.Time]StatMinutely)
		}
		bucketsByMonitor[row.MonitorURLID][row.BucketAt.UTC()] = row
	}

	// Для каждого монитора заполняем фиксированный массив из 30 минутных слотов.
	for _, monitorID := range monitorIDs {
		createdAt := createdAtByID[monitorID]
		createdMinute := createdAt.Truncate(time.Minute)
		buckets := bucketsByMonitor[monitorID]

		bars := make([]UptimeHistoryBar, 0, uptimeHistoryMinutes)
		for i := 0; i < uptimeHistoryMinutes; i++ {
			bucketAt := windowStart.Add(time.Duration(i) * time.Minute)
			if bucketAt.Before(createdMinute) {
				// Монитор ещё не существовал — данных за эту минуту быть не может.
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarNoData})
				continue
			}

			bucket, ok := buckets[bucketAt.UTC()]
			if !ok || bucket.TotalSeconds == 0 {
				// Нет bucket или не было наблюдений в эту минуту.
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarNoData})
				continue
			}

			switch {
			case bucket.UpSeconds >= bucket.TotalSeconds:
				// Вся наблюдаемая минута — uptime.
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarUp})
			case bucket.UpSeconds == 0:
				// Наблюдали минуту, но без секунд up — полный downtime.
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarDown})
			default:
				// Часть минуты up, часть down (смена статуса внутри bucket).
				bars = append(bars, UptimeHistoryBar{BucketAt: bucketAt, State: UptimeBarMixed})
			}
		}
		result[monitorID] = bars
	}

	return result, nil
}

// loadUptimeSummary считает UptimeSummary за period, если монитор существовал достаточно долго;
// иначе возвращает пустую сводку. since обрезается по createdAt.
func loadUptimeSummary(db *gorm.DB, monitorID uint, createdAt, now time.Time, period time.Duration, granularity uptimeGranularity) (UptimeSummary, error) {
	if !uptimePeriodEligible(createdAt, now, period) {
		// Монитор моложе окна — показываем «нет данных», не частичный процент.
		return UptimeSummary{}, nil
	}
	since := effectiveSince(createdAt, now.Add(-period))
	return sumUptimeBuckets(db, monitorID, since, granularity)
}

// uptimePeriodEligible сообщает, существовал ли монитор в течение полного отчётного периода.
func uptimePeriodEligible(createdAt, now time.Time, period time.Duration) bool {
	return !createdAt.After(now.Add(-period))
}

// effectiveSince возвращает более позднее из начала отчётного окна и времени создания монитора.
func effectiveSince(createdAt, since time.Time) time.Time {
	if createdAt.After(since) {
		// Монитор создан позже начала окна — считаем только с момента создания.
		return createdAt
	}
	return since
}

// sumUptimeBuckets суммирует up_seconds и total_seconds одного монитора из stat-таблицы
// выбранной гранулярности начиная с since (включительно).
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

// sumUptimeBucketsForMonitors — пакетный аналог loadUptimeSummary: для каждого monitorID
// отфильтровывает «слишком молодые» мониторы и агрегирует bucket одним запросом.
func sumUptimeBucketsForMonitors(db *gorm.DB, monitorIDs []uint, createdAtByID map[uint]time.Time, now time.Time, period time.Duration, granularity uptimeGranularity) (map[uint]UptimeSummary, error) {
	result := make(map[uint]UptimeSummary, len(monitorIDs))
	eligibleIDs := make([]uint, 0, len(monitorIDs))
	sinceByID := make(map[uint]time.Time, len(monitorIDs))
	windowSince := now.Add(-period)

	// Разделяем мониторы на «достаточно старые» и «ещё молодые» для данного окна.
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

	// Инициализируем eligible нулями — LEFT JOIN вернёт 0, если bucket не было.
	for _, id := range eligibleIDs {
		result[id] = UptimeSummary{}
	}
	for _, row := range rows {
		result[row.MonitorURLID] = row.summary()
	}
	return result, nil
}

// sumUptimeBucketsForEligibleMonitors агрегирует bucket uptime для многих мониторов одним SQL-запросом.
// db — дескриптор базы данных; tableName — доверенное имя stat-таблицы; monitorIDs — подходящие мониторы;
// sinceByID содержит нижнюю границу bucket для каждого монитора.
func sumUptimeBucketsForEligibleMonitors(db *gorm.DB, tableName string, monitorIDs []uint, sinceByID map[uint]time.Time) ([]uptimeMonitorSummaryRow, error) {
	// CTE requested: пары (monitor_id, since_at) — у каждого монитора свой нижний cutoff bucket.
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
		-- LEFT JOIN: мониторы без bucket за окно остаются в результате с SUM = 0.
		GROUP BY requested.monitor_url_id
	`, valuesSQL, tableName)

	var rows []uptimeMonitorSummaryRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// uptimeBucketRequestValues формирует список VALUES для окон uptime по мониторам.
// monitorIDs выводятся по порядку; sinceByID задаёт нижнюю границу bucket для каждого id.
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

// tableNameForGranularity возвращает имя stat-таблицы для чтения bucket данной гранулярности.
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

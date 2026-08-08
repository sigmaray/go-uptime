package worker

import (
	"time"

	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

// backfillUptimeStatsIfNeeded восстанавливает uptime-бакеты из истории проверок, когда таблицы статистики пусты.
// Запускается один раз при старте worker (до основного scheduling loop), если stat_minutely пуста —
// типичный случай после апгрейда на версию с агрегатами: существующие мониторы получают историю без ручной миграции.
func (w *MonitorWorker) backfillUptimeStatsIfNeeded() {
	var count int64
	if err := w.db.Model(&models.StatMinutely{}).Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("failed to count uptime stats")
		return
	}
	if count > 0 {
		// Агрегаты уже есть — backfill после апгрейда не нужен.
		return
	}

	// Первый запуск на версии с uptime-таблицами — восстанавливаем из monitor_checks.
	if err := models.BackfillUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to backfill uptime stats")
	}
}

// maintenanceLoop удаляет старые записи по собственному ticker (раз в минуту), отдельно от scheduling проверок.
// Retention и prune не должны блокировать hot path claim/check/persist — поэтому отдельная goroutine и ticker.
// Закрывает maintenanceDone при сигнале stop, чтобы Stop мог дождаться чистого завершения.
func (w *MonitorWorker) maintenanceLoop() {
	defer close(w.maintenanceDone)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.runMaintenance()
		case <-w.stop:
			// Не ждём текущий prune — выходим сразу по сигналу Stop.
			return
		}
	}
}

// runMaintenance удаляет incidents, историю проверок и uptime-бакеты по правилам retention из config.
// Читает IncidentRetentionDays и MaxResolvedIncidentsPerMonitor; также вызывает PruneMonitorChecks и PruneUptimeStats.
// Ничего не делает при паузе worker или при отсутствии подключения к БД или config.
func (w *MonitorWorker) runMaintenance() {
	if w.Paused() || w.db == nil || w.cfg == nil {
		// На паузе или без конфига — retention не трогаем (e2e, тесты с nil cfg).
		return
	}

	if err := models.PruneIncidents(w.db, w.cfg.IncidentRetentionDays, w.cfg.MaxResolvedIncidentsPerMonitor); err != nil {
		log.Error().Err(err).Msg("failed to prune incidents")
	}

	if err := models.PruneMonitorChecks(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune monitor checks")
	}

	if err := models.PruneUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune uptime stats")
	}
}

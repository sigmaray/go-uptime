// Package worker выполняет фоновые HTTP-проверки мониторируемых URL.
package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go-uptime/config"
	"go-uptime/internal/applog"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const requestTimeout = 15 * time.Second

// MonitorWorker периодически проверяет URL из базы данных.
type MonitorWorker struct {
	db     *gorm.DB
	cfg    *config.Config
	client *http.Client
	stop   chan struct{}
}

// New создаёт новый экземпляр фонового воркера мониторинга.
func New(db *gorm.DB, cfg *config.Config) *MonitorWorker {
	return &MonitorWorker{
		db:  db,
		cfg: cfg,
		client: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		stop: make(chan struct{}),
	}
}

// Start запускает цикл проверок в отдельной горутине.
func (w *MonitorWorker) Start() {
	go w.backfillUptimeStatsIfNeeded()
	go w.loop()
}

func (w *MonitorWorker) backfillUptimeStatsIfNeeded() {
	var count int64
	if err := w.db.Model(&models.StatMinutely{}).Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("failed to count uptime stats")
		return
	}
	if count > 0 {
		return
	}

	if err := models.BackfillUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to backfill uptime stats")
		applog.AddError("failed to backfill uptime stats", err.Error())
	}
}

// Stop останавливает воркер.
func (w *MonitorWorker) Stop() {
	close(w.stop)
}

func (w *MonitorWorker) loop() {
	interval := time.Duration(models.GetCheckIntervalSeconds(w.db, w.cfg.CheckIntervalSeconds)) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.runOnce()

	for {
		select {
		case <-ticker.C:
			newInterval := time.Duration(models.GetCheckIntervalSeconds(w.db, w.cfg.CheckIntervalSeconds)) * time.Second
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
			w.runOnce()
		case <-w.stop:
			return
		}
	}
}

func (w *MonitorWorker) runOnce() {
	var monitors []models.MonitorURL
	if err := w.db.Find(&monitors).Error; err != nil {
		log.Error().Err(err).Msg("failed to load monitor urls")
		applog.AddError("failed to load monitor urls", err.Error())
		return
	}

	for _, monitor := range monitors {
		w.checkMonitor(monitor)
	}

	if err := models.PruneIncidents(w.db, w.cfg.IncidentRetentionDays, w.cfg.MaxResolvedIncidentsPerMonitor); err != nil {
		log.Error().Err(err).Msg("failed to prune incidents")
		applog.AddError("failed to prune incidents", err.Error())
	}

	if err := models.PruneMonitorChecks(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune monitor checks")
		applog.AddError("failed to prune monitor checks", err.Error())
	}

	if err := models.PruneUptimeStats(w.db); err != nil {
		log.Error().Err(err).Msg("failed to prune uptime stats")
		applog.AddError("failed to prune uptime stats", err.Error())
	}
}

func (w *MonitorWorker) checkMonitor(monitor models.MonitorURL) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, monitor.URL, nil)
	if err != nil {
		w.markDown(monitor, err.Error())
		return
	}
	req.Header.Set("User-Agent", "go-uptime-monitor/1.0")

	resp, err := w.client.Do(req)
	now := time.Now()
	if err != nil {
		w.markDown(monitor, err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		w.markDown(monitor, fmt.Sprintf("unexpected status code: %d", resp.StatusCode))
		return
	}

	w.markUp(monitor, now)
}

func (w *MonitorWorker) markUp(monitor models.MonitorURL, checkedAt time.Time) {
	wasDown := monitor.IsUp == nil || !*monitor.IsUp

	updates := map[string]interface{}{
		"is_up":           true,
		"last_checked_at": checkedAt,
		"last_error":      "",
	}
	if err := w.db.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error; err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to update monitor status")
		return
	}

	w.recordCheck(monitor.ID, checkedAt, true)

	openIncident, err := models.FindOpenIncident(w.db, monitor.ID)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to find open incident")
		return
	}
	if openIncident != nil {
		now := checkedAt
		if err := w.db.Model(openIncident).Update("resolved_at", now).Error; err != nil {
			log.Error().Err(err).Uint("incident_id", openIncident.ID).Msg("failed to resolve incident")
		}
	}

	if wasDown {
		applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is UP", models.MonitorDisplayName(monitor), monitor.URL))
	}
}

func (w *MonitorWorker) markDown(monitor models.MonitorURL, errMsg string) {
	wasUp := monitor.IsUp == nil || *monitor.IsUp
	now := time.Now()
	updates := map[string]interface{}{
		"is_up":           false,
		"last_checked_at": now,
		"last_error":      errMsg,
	}
	if err := w.db.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error; err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to update monitor status")
		return
	}

	w.recordCheck(monitor.ID, now, false)

	openIncident, err := models.FindOpenIncident(w.db, monitor.ID)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to find open incident")
		return
	}
	if openIncident == nil {
		incident := models.Incident{
			MonitorURLID: monitor.ID,
			StartedAt:    now,
			ErrorMessage: errMsg,
		}
		if err := w.db.Create(&incident).Error; err != nil {
			log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to create incident")
		}
	} else if openIncident.ErrorMessage != errMsg {
		_ = w.db.Model(openIncident).Update("error_message", errMsg).Error
	}

	if wasUp {
		applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is DOWN: %s", models.MonitorDisplayName(monitor), monitor.URL, errMsg))
	}
}

func (w *MonitorWorker) recordCheck(monitorID uint, checkedAt time.Time, isUp bool) {
	if err := models.RecordMonitorCheck(w.db, monitorID, checkedAt, isUp); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitorID).Msg("failed to record monitor check")
	}
}

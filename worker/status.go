package worker

import (
	"fmt"
	"time"

	"go-uptime/internal/applog"
	"go-uptime/internal/services/monitor"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

func (w *MonitorWorker) markUp(mon models.MonitorURL, checkedAt time.Time, responseTimeMs *int) {
	svc := monitor.NewService(w.db)
	wasDown, err := svc.MarkUp(mon, checkedAt, responseTimeMs)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", mon.ID).Msg("failed to process up state transition")
		return
	}

	if wasDown {
		applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is UP", models.MonitorDisplayName(mon), mon.URL))
		w.enqueueNotification(mon, true, "")
	}
}

func (w *MonitorWorker) markDown(mon models.MonitorURL, errMsg string, responseTimeMs *int) {
	now := time.Now()
	svc := monitor.NewService(w.db)
	wasUp, err := svc.MarkDown(mon, now, errMsg, responseTimeMs)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", mon.ID).Msg("failed to process down state transition")
		return
	}

	if wasUp {
		applog.AddEvent("monitor", fmt.Sprintf("Monitor %q (%s) is DOWN: %s", models.MonitorDisplayName(mon), mon.URL, errMsg))
		w.enqueueNotification(mon, false, errMsg)
	}
}

package worker

import (
	"fmt"

	"go-uptime/internal/applog"
	"go-uptime/internal/notify"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

// notifyJob is one monitor status-change alert to send off the check path.
type notifyJob struct {
	monitor models.MonitorURL
	isUp    bool
	errMsg  string
}

// notifyLoop sends queued status-change alerts so SMTP/Telegram never block HTTP checks.
func (w *MonitorWorker) notifyLoop() {
	defer close(w.notifyDone)

	for job := range w.notifyJobs {
		w.deliverNotification(job.monitor, job.isUp, job.errMsg)
	}
}

// enqueueNotification queues a status-change alert without waiting for delivery.
// monitor is the monitor that changed state; isUp is the new availability;
// errMsg is the down reason (empty when the monitor recovered).
func (w *MonitorWorker) enqueueNotification(monitor models.MonitorURL, isUp bool, errMsg string) {
	if !monitor.NotifyTelegram && !monitor.NotifySMTP {
		return
	}

	job := notifyJob{
		monitor: monitor,
		isUp:    isUp,
		errMsg:  errMsg,
	}
	select {
	case w.notifyJobs <- job:
	default:
		log.Warn().
			Uint("monitor_id", monitor.ID).
			Msg("notification queue full, enqueueing asynchronously")
		applog.AddError(
			"notification queue full",
			fmt.Sprintf("monitor_id=%d alert delayed", monitor.ID),
		)
		go func() {
			w.notifyJobs <- job
		}()
	}
}

// deliverNotification sends one queued alert via the injected sender or the default path.
// monitor is the monitor that changed state; isUp is the new availability;
// errMsg is the down reason (empty when the monitor recovered).
func (w *MonitorWorker) deliverNotification(monitor models.MonitorURL, isUp bool, errMsg string) {
	if w.notifySender != nil {
		w.notifySender(monitor, isUp, errMsg)
		return
	}
	w.sendNotifications(monitor, isUp, errMsg)
}

// sendNotifications sends status-change notifications when channels are configured and enabled for the monitor.
// monitor is the monitor that changed; isUp is the new state; errMsg explains a down transition.
func (w *MonitorWorker) sendNotifications(monitor models.MonitorURL, isUp bool, errMsg string) {
	if !monitor.NotifyTelegram && !monitor.NotifySMTP {
		return
	}

	settings, err := models.LoadNotificationSettings(w.db)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to load notification settings")
		applog.AddError("failed to load notification settings", err.Error())
		return
	}

	change := notify.MonitorStateChange{
		DisplayName: models.MonitorDisplayName(monitor),
		URL:         monitor.URL,
		IsUp:        isUp,
		Error:       errMsg,
	}
	if err := notify.SendMonitorStateChange(settings, monitor, change); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to send monitor notification")
		applog.AddError("failed to send monitor notification", err.Error())
	}
}

package worker

import (
	"go-uptime/internal/notify"
	"go-uptime/models"

	"github.com/rs/zerolog/log"
)

// notifyJob — одно оповещение об изменении статуса монитора для отправки вне пути проверки.
type notifyJob struct {
	monitor models.MonitorURL
	isUp    bool
	errMsg  string
}

// notifyLoop отправляет оповещения из очереди, чтобы SMTP/Telegram не блокировали HTTP-проверки.
func (w *MonitorWorker) notifyLoop() {
	defer close(w.notifyDone)

	// Читаем до закрытия notifyJobs — Stop закрывает channel после batch flush.
	for job := range w.notifyJobs {
		w.deliverNotification(job.monitor, job.isUp, job.errMsg)
	}
}

// enqueueNotification ставит оповещение об изменении статуса в очередь без ожидания доставки.
// monitor — монитор, у которого изменился статус; isUp — новая доступность;
// errMsg — причина падения (пустая, если монитор восстановился).
func (w *MonitorWorker) enqueueNotification(monitor models.MonitorURL, isUp bool, errMsg string) {
	// Нет включённых channel — не засоряем очередь и не читаем settings.
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
		// Успешно поставили в очередь — notifyLoop доставит асинхронно.
	default:
		// Очередь полна — намеренно отбрасываем alert, а не блокируем путь проверки/persist.
		log.Warn().
			Uint("monitor_id", monitor.ID).
			Msg("notification queue full, dropping alert")
	}
}

// deliverNotification отправляет одно оповещение из очереди через injected sender или путь по умолчанию.
// monitor — монитор, у которого изменился статус; isUp — новая доступность;
// errMsg — причина падения (пустая, если монитор восстановился).
func (w *MonitorWorker) deliverNotification(monitor models.MonitorURL, isUp bool, errMsg string) {
	if w.notifySender != nil {
		// Тесты и инъекция — обход реального SMTP/Telegram.
		w.notifySender(monitor, isUp, errMsg)
		return
	}
	w.sendNotifications(monitor, isUp, errMsg)
}

// sendNotifications отправляет оповещения об изменении статуса, если channel настроены и включены для монитора.
// monitor — монитор, у которого изменился статус; isUp — новое состояние; errMsg объясняет переход в down.
func (w *MonitorWorker) sendNotifications(monitor models.MonitorURL, isUp bool, errMsg string) {
	if !monitor.NotifyTelegram && !monitor.NotifySMTP {
		return
	}

	// Настройки читаем на каждое job, чтобы изменения в admin UI применялись без перезапуска worker.
	settings, err := models.LoadNotificationSettings(w.db)
	if err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to load notification settings")
		return
	}

	change := notify.MonitorStateChange{
		DisplayName: models.MonitorDisplayName(monitor),
		URL:         monitor.URL,
		IsUp:        isUp,
		Error:       errMsg,
	}
	// Shoutrrr/SMTP/Telegram — ошибка логируется, goroutine notifyLoop продолжает работу.
	if err := notify.SendMonitorStateChange(settings, monitor, change); err != nil {
		log.Error().Err(err).Uint("monitor_id", monitor.ID).Msg("failed to send monitor notification")
	}
}

// Package worker выполняет фоновые HTTP-проверки отслеживаемых URL.
package worker

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go-uptime/config"
	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"gorm.io/gorm"
)

// DefaultCheckConcurrency используется, когда config не задаёт или задаёт некорректное значение concurrency.
const DefaultCheckConcurrency = 150

// notifyQueueSize — размер буфера для асинхронных оповещений об изменении статуса.
const notifyQueueSize = 256

// resultQueueSize — размер буфера для завершённых проверок, ожидающих flush в БД.
const resultQueueSize = 2048

// Stats — снимок метрик проверок мониторов и очереди notify на текущий момент.
type Stats struct {
	// DueThisWave — сколько захваченных мониторов ещё не завершили проверку.
	DueThisWave int
	// InFlight — сколько HTTP-проверок выполняется прямо сейчас.
	InFlight int
	// WaitingForSlot — сколько захваченных мониторов всё ещё ждут слота concurrency.
	WaitingForSlot int
	// MaxConcurrency — настроенный лимит одновременных HTTP-проверок.
	MaxConcurrency int
	// NotifyQueued — сколько оповещений об изменении статуса находится в channel notify.
	NotifyQueued int
	// NotifyCapacity — размер буфера channel notify.
	NotifyCapacity int
	// ResultQueued — завершённые проверки, ожидающие сохранения (channel плюс остатки batch в памяти).
	ResultQueued int
	// ResultCapacity — размер буфера channel persist.
	ResultCapacity int
}

// checkResult представляет результат одной HTTP-проверки для batch-записи в БД.
type checkResult struct {
	// monitor — снимок строки monitor_urls на момент захвата проверки.
	monitor models.MonitorURL
	// isUp — результат доступности проверки.
	isUp bool
	// errMsg — текст ошибки проверки; пустой при успешной проверке.
	errMsg string
	// elapsed — длительность проверки в миллисекундах; nil, если неизвестна.
	elapsed *int
	// persistAttempts — сколько раз результат был повторно поставлен в очередь после неудачного flush.
	persistAttempts int
}

// MonitorWorker периодически проверяет URL из базы данных.
type MonitorWorker struct {
	db               *gorm.DB
	cfg              *config.Config
	client           *http.Client
	checkConcurrency int
	// checkSem ограничивает число одновременных HTTP-проверок между перекрывающимися волнами claim.
	checkSem chan struct{}
	// notifyJobs буферизует оповещения об изменении статуса для отправки через внешние channel (например Telegram/SMTP).
	notifyJobs chan notifyJob
	// resultJobs буферизует завершённые HTTP-проверки для batch-сохранения в БД.
	resultJobs chan checkResult
	// notifySender доставляет одно оповещение; nil означает путь Shoutrrr по умолчанию.
	notifySender func(monitor models.MonitorURL, isUp bool, errMsg string)

	stop            chan struct{}
	loopDone        chan struct{}
	notifyDone      chan struct{}
	maintenanceDone chan struct{}
	// batchDone закрывается, когда goroutine batchResultsLoop полностью завершилась.
	batchDone chan struct{}
	// wavesWG отслеживает запущенные goroutine проверок, чтобы Stop мог дождаться их завершения.
	wavesWG  sync.WaitGroup
	started  atomic.Bool
	stopOnce sync.Once
	// paused пропускает проверки due-мониторов и maintenance, оставляя Running() true.
	// Используется Playwright test API, чтобы e2e-очистки не гонялись с выполняющимися HTTP-проверками.
	paused atomic.Bool

	// waveDue — сколько захваченных мониторов ещё не завершили проверку.
	waveDue atomic.Int64
	// waveStarted считает мониторы, получившие слот concurrency и ещё не завершившие проверку.
	waveStarted atomic.Int64
	// inFlight считает выполняющиеся HTTP-проверки.
	inFlight atomic.Int64
	// persistBacklog — сколько завершённых проверок ждут успешного flush в batch loop.
	persistBacklog atomic.Int64
}

// New создаёт новый экземпляр фонового worker мониторинга.
// db — GORM-подключение для загрузки мониторов и сохранения результатов проверок.
// cfg задаёт настройки retention и максимальное число одновременных HTTP-проверок.
func New(db *gorm.DB, cfg *config.Config) *MonitorWorker {
	// Берём дефолтный лимит параллельных HTTP-проверок.
	concurrency := DefaultCheckConcurrency
	if cfg != nil && cfg.CheckConcurrency > 0 {
		// Явное значение из config перекрывает дефолт.
		concurrency = cfg.CheckConcurrency
	}

	return &MonitorWorker{
		db:               db,
		cfg:              cfg,
		checkConcurrency: concurrency,
		// Semaphore с ёмкостью = concurrency ограничивает in-flight HTTP между волнами.
		checkSem: make(chan struct{}, concurrency),
		// HTTP-клиент с тем же лимитом, чтобы transport не открывал лишние соединения.
		client: urlcheck.NewClient(concurrency),
		// Буферизованные channel — горутины проверок не блокируются на persist/notify.
		notifyJobs: make(chan notifyJob, notifyQueueSize),
		resultJobs: make(chan checkResult, resultQueueSize),
		// stop закрывается один раз в Stop(); done-channel сигнализируют о выходе goroutine.
		stop:            make(chan struct{}),
		loopDone:        make(chan struct{}),
		notifyDone:      make(chan struct{}),
		maintenanceDone: make(chan struct{}),
		batchDone:       make(chan struct{}),
	}
}

// Running сообщает, активен ли цикл проверки мониторов и не был ли он остановлен.
// Возвращает false для nil worker, до Start и после начала остановки через Stop.
// Worker на паузе всё ещё считается running, чтобы /health оставался healthy во время e2e.
func (w *MonitorWorker) Running() bool {
	// nil или ещё не вызван Start — worker не активен.
	if w == nil || !w.started.Load() {
		return false
	}
	// Закрытый stop означает, что Stop уже начал graceful shutdown.
	select {
	case <-w.stop:
		return false
	default:
		return true
	}
}

// Pause прекращает планирование новых проверок мониторов и волн maintenance.
// Уже запущенные проверки из начавшейся волны не отменяются.
func (w *MonitorWorker) Pause() {
	if w == nil {
		return
	}
	// Атомарный флаг: runDueMonitors и maintenance читают его без mutex.
	w.paused.Store(true)
}

// Resume снова разрешает циклу проверок планировать проверки мониторов после Pause.
func (w *MonitorWorker) Resume() {
	if w == nil {
		return
	}
	// Снимаем паузу — следующий tick снова захватит due-мониторы.
	w.paused.Store(false)
}

// Paused сообщает, подавлено ли сейчас планирование новых волн проверок.
func (w *MonitorWorker) Paused() bool {
	return w != nil && w.paused.Load()
}

// Stats возвращает счётчики волн проверок, очереди persist и очереди уведомлений для ops-страниц.
func (w *MonitorWorker) Stats() Stats {
	if w == nil {
		return Stats{}
	}

	// waveDue — все захваченные, но ещё не завершившие enqueue.
	due := int(w.waveDue.Load())
	// waveStarted — уже получили слот semaphore и выполняют HTTP.
	started := int(w.waveStarted.Load())
	inFlight := int(w.inFlight.Load())
	// Остальные ждут свободного слота в checkSem.
	waiting := due - started
	if waiting < 0 {
		// Защита от рассинхрона счётчиков при гонках defer.
		waiting = 0
	}

	return Stats{
		DueThisWave:    due,
		InFlight:       inFlight,
		WaitingForSlot: waiting,
		MaxConcurrency: w.checkConcurrency,
		NotifyQueued:   len(w.notifyJobs),
		NotifyCapacity: notifyQueueSize,
		// persistBacklog — batch в памяти batch loop, ещё не записанный в БД.
		ResultQueued:   len(w.resultJobs) + int(w.persistBacklog.Load()),
		ResultCapacity: cap(w.resultJobs),
	}
}

// Start запускает цикл проверок, batch writer, maintenance и отправку уведомлений.
func (w *MonitorWorker) Start() {
	// Повторный Start безопасно игнорируется — CAS защищает от двойного запуска goroutine.
	if !w.started.CompareAndSwap(false, true) {
		return
	}
	// Одноразовый backfill uptime при пустой stat_minutely (апгрейд без миграции данных).
	go w.backfillUptimeStatsIfNeeded()
	// Асинхронная доставка Telegram/SMTP — не блокирует hot path проверок.
	go w.notifyLoop()
	// Batch writer: собирает resultJobs и пишет в PostgreSQL пакетами.
	go w.batchResultsLoop()
	// Retention/prune раз в минуту, отдельно от секундного scheduling.
	go w.maintenanceLoop()
	// Основной цикл: раз в секунду claim due-мониторов и dispatch HTTP.
	go w.loop()
}

// Stop останавливает цикл проверок, сливает очереди результатов и уведомлений, затем возвращается.
func (w *MonitorWorker) Stop() {
	w.stopOnce.Do(func() {
		if !w.started.Load() {
			return
		}
		// Сигнал всем goroutine: прекратить планирование новой работы.
		close(w.stop)
		// Ждём loop: он дождётся wavesWG и выйдет.
		<-w.loopDone
		// Maintenance тоже слушает stop и завершится сам.
		<-w.maintenanceDone
		// Закрываем resultJobs — batch loop сольёт остаток и закроет batchDone.
		close(w.resultJobs)
		<-w.batchDone
		// После persist закрываем notify и ждём доставки уже поставленных job.
		close(w.notifyJobs)
		<-w.notifyDone
	})
}

// loop захватывает due-мониторы по ticker раз в секунду, пока не закрыт stop.
func (w *MonitorWorker) loop() {
	defer close(w.loopDone)

	// Раз в секунду — компромисс между latency due-мониторов и нагрузкой на БД claim.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Первый claim сразу при старте, не ждём первого tick.
	w.runDueMonitors()

	for {
		select {
		case <-ticker.C:
			w.runDueMonitors()
		case <-w.stop:
			// Ждём уже запущенные проверки, чтобы resultJobs не закрыли преждевременно.
			w.wavesWG.Wait()
			return
		}
	}
}

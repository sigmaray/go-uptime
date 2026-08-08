package worker

import (
	"sync"

	"go-uptime/models"
)

// runChecksConcurrently — тестовый helper, который ждёт завершения всех проверок.
// В production вместо него используется MonitorWorker.dispatchChecks.
// monitors — набор мониторов для проверки.
// maxConcurrent ограничивает число одновременных вызовов checkFn; значения ниже 1 становятся 1.
// checkFn выполняет одну проверку и должна быть безопасна для concurrent-вызовов.
func runChecksConcurrently(monitors []models.MonitorURL, maxConcurrent int, checkFn func(models.MonitorURL)) {
	if len(monitors) == 0 {
		return
	}
	if maxConcurrent < 1 {
		// Некорректный лимит — хотя бы одна goroutine одновременно.
		maxConcurrent = 1
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, monitor := range monitors {
		wg.Add(1)
		// Захватываем слот до старта goroutine — не создаём лишних ожидающих.
		sem <- struct{}{}
		go func(m models.MonitorURL) {
			defer wg.Done()
			defer func() { <-sem }()
			checkFn(m)
		}(monitor)
	}

	// Тест ждёт завершения всех проверок — в отличие от dispatchChecks в production.
	wg.Wait()
}

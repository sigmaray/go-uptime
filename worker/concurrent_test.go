package worker

import (
	"sync"

	"go-uptime/models"
)

// runChecksConcurrently is a test-only helper that waits for every check to finish.
// Production scheduling uses MonitorWorker.dispatchChecks instead.
// monitors is the set of monitors to check.
// maxConcurrent caps simultaneous checkFn calls; values below 1 become 1.
// checkFn runs one probe and must be concurrency-safe.
func runChecksConcurrently(monitors []models.MonitorURL, maxConcurrent int, checkFn func(models.MonitorURL)) {
	if len(monitors) == 0 {
		return
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, monitor := range monitors {
		wg.Add(1)
		sem <- struct{}{}
		go func(m models.MonitorURL) {
			defer wg.Done()
			defer func() { <-sem }()
			checkFn(m)
		}(monitor)
	}

	wg.Wait()
}

// Package urlcheck проверяет HTTP(S) URL по тем же правилам успеха, что и worker мониторов.
package urlcheck

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// RequestTimeout — дедлайн на один запрос при проверке мониторов.
const RequestTimeout = 30 * time.Second

// MaxProbeBodyBytes ограничивает объём тела HTTP-ответа, считываемого при проверке.
// Более крупные тела отбрасываются после этого лимита, чтобы медленные/огромные загрузки
// не занимали слот проверки.
const MaxProbeBodyBytes = 64 * 1024

// browserLikeUserAgent имитирует обычный десктопный Chrome, чтобы WAF / bot-фильтры
// реже отклоняли проверки только из-за явного User-Agent монитора.
const browserLikeUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// browserLikeAccept — типичный заголовок Accept браузера для навигации к документу верхнего уровня.
const browserLikeAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"

// browserLikeAcceptLanguage предпочитает английский и русский, чтобы локализованные сайты
// по-прежнему воспринимали запрос как обычный визит браузера от международного клиента.
const browserLikeAcceptLanguage = "en-US,en;q=0.9,ru;q=0.8"

// Result — результат одной проверки URL.
type Result struct {
	// URL — адрес, который был проверен.
	URL string
	// Up равен true, когда статус ответа в диапазоне [200, 400).
	Up bool
	// StatusCode — HTTP-статус при получении ответа; ноль при ошибках транспорта.
	StatusCode int
	// ErrMsg описывает причину неудачи проверки, когда Up равен false.
	ErrMsg string
	// DurationMs — длительность проверки в миллисекундах.
	DurationMs int64
}

// IsUpStatus сообщает, считается ли statusCode «up» для проверок мониторов.
// statusCode — HTTP-статус ответа проверенного URL.
func IsUpStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 400
}

// SetBrowserLikeHeaders добавляет заголовки запроса, похожие на обычный браузерный GET.
// req — исходящий запрос проверки, который отправит HTTP-клиент.
// Это снижает число ложных «down» из-за простых bot-фильтров, отклоняющих кастомные User-Agent мониторов.
// Meta-сайты (например WhatsApp) возвращают HTTP 400 для User-Agent Chrome без
// заголовков навигации Sec-Fetch-*, поэтому они тоже добавляются.
func SetBrowserLikeHeaders(req *http.Request) {
	// Имитация Chrome на Linux — снижает блокировки bot-фильтрами.
	req.Header.Set("User-Agent", browserLikeUserAgent)
	req.Header.Set("Accept", browserLikeAccept)
	req.Header.Set("Accept-Language", browserLikeAcceptLanguage)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	// Sec-Fetch-* нужны некоторым сайтам (WhatsApp и др.) — иначе HTTP 400.
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
}

// NewClient создаёт HTTP-клиент, подходящий для проверок мониторов.
// maxConcurrent задаёт размер пулов соединений; значения ниже 1 становятся 1.
func NewClient(maxConcurrent int) *http.Client {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &http.Client{
		Timeout:   RequestTimeout,
		Transport: newTransport(maxConcurrent),
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			// Не следуем бесконечным redirect-цепочкам — максимум 5 переходов.
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// newTransport создаёт HTTP transport, рассчитанный на параллельные проверки.
// maxConcurrent используется для размера пулов соединений.
func newTransport(maxConcurrent int) *http.Transport {
	idleConns := maxConcurrent * 2
	if idleConns < 100 {
		// Минимальный пул idle-соединений для типичной нагрузки worker.
		idleConns = 100
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          idleConns,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// Probe выполняет GET к rawURL и сообщает, считается ли сайт доступным.
// ctx отменяет проверку; client выполняет HTTP-запрос (не должен быть nil).
// rawURL — абсолютный HTTP или HTTPS адрес для проверки.
func Probe(ctx context.Context, client *http.Client, rawURL string) Result {
	result := Result{URL: rawURL}
	start := time.Now()

	// ctx отменяет запрос (таймаут клиента или отмена bulk-verify).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		// Невалидный URL или ошибка сборки запроса — без HTTP-статуса.
		result.DurationMs = time.Since(start).Milliseconds()
		result.ErrMsg = err.Error()
		return result
	}
	// Браузероподобные заголовки снижают ложные «down» от простых bot-фильтров.
	SetBrowserLikeHeaders(req)

	resp, err := client.Do(req)
	result.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		// Таймаут, DNS, TLS, connection reset и т.д. — транспортная ошибка, Up=false.
		result.ErrMsg = err.Error()
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	// Считываем тело до лимита и отбрасываем — иначе conn не вернётся в pool.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxProbeBodyBytes))

	result.StatusCode = resp.StatusCode
	// «Up» — любой 2xx/3xx; 4xx/5xx считаем недоступностью цели.
	if !IsUpStatus(resp.StatusCode) {
		result.ErrMsg = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		return result
	}

	result.Up = true
	return result
}

// ProbeAll проверяет каждый URL параллельно и возвращает результаты в том же порядке, что и urls.
// ctx отменяет незавершённые проверки; client выполняет HTTP-запросы.
// urls — абсолютные HTTP/HTTPS адреса для проверки.
// maxConcurrent ограничивает число одновременных проверок; значения ниже 1 становятся 1.
func ProbeAll(ctx context.Context, client *http.Client, urls []string, maxConcurrent int) []Result {
	if len(urls) == 0 {
		return nil
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	results := make([]Result, len(urls))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, rawURL := range urls {
		wg.Add(1)
		sem <- struct{}{} // захват слота семафора
		go func(idx int, u string) {
			defer wg.Done()
			defer func() { <-sem }() // освобождение слота
			// results[idx] сохраняет порядок входного слайса urls.
			results[idx] = Probe(ctx, client, u)
		}(i, rawURL)
	}

	wg.Wait()
	return results
}

// UnavailableURLs возвращает результаты проверок, которые не «up», сохраняя порядок входа.
// results — исходы Probe или ProbeAll.
func UnavailableURLs(results []Result) []Result {
	out := make([]Result, 0)
	for _, r := range results {
		if !r.Up {
			// Сохраняем порядок входа — удобно для сообщений об ошибках bulk-create.
			out = append(out, r)
		}
	}
	return out
}

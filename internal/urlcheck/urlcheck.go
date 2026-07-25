// Package urlcheck probes HTTP(S) URLs with the same success rules as the monitor worker.
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

// RequestTimeout is the per-request deadline used for monitor probes.
const RequestTimeout = 30 * time.Second

// browserLikeUserAgent mimics a common desktop Chrome browser so WAF / bot filters
// are less likely to reject checks solely because of an obvious monitor User-Agent.
const browserLikeUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// browserLikeAccept is a typical browser Accept header for a top-level document navigation.
const browserLikeAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"

// browserLikeAcceptLanguage prefers English and Russian so localized sites still treat the
// request as a normal browser visit from an international client.
const browserLikeAcceptLanguage = "en-US,en;q=0.9,ru;q=0.8"

// Result is the outcome of a single URL probe.
type Result struct {
	// URL is the address that was probed.
	URL string
	// Up is true when the response status is in [200, 400).
	Up bool
	// StatusCode is the HTTP status when a response was received; zero on transport errors.
	StatusCode int
	// ErrMsg describes why the probe failed when Up is false.
	ErrMsg string
	// DurationMs is how long the probe took in milliseconds.
	DurationMs int64
}

// IsUpStatus reports whether statusCode counts as "up" for monitor checks.
// statusCode is the HTTP response status from the probed URL.
func IsUpStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 400
}

// SetBrowserLikeHeaders attaches request headers that resemble a normal browser GET.
// req is the outbound probe request that will be sent by the HTTP client.
// This reduces false downs from simple bot filters that reject custom monitor User-Agents.
// Meta properties (for example WhatsApp) return HTTP 400 for a Chrome User-Agent without
// Sec-Fetch-* navigation headers, so those are included as well.
func SetBrowserLikeHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserLikeUserAgent)
	req.Header.Set("Accept", browserLikeAccept)
	req.Header.Set("Accept-Language", browserLikeAcceptLanguage)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
}

// NewClient builds an HTTP client suitable for monitor probes.
// maxConcurrent sizes connection pools; values below 1 become 1.
func NewClient(maxConcurrent int) *http.Client {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &http.Client{
		Timeout:   RequestTimeout,
		Transport: newTransport(maxConcurrent),
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// newTransport builds an HTTP transport sized for concurrent probes.
// maxConcurrent is used to size connection pools.
func newTransport(maxConcurrent int) *http.Transport {
	idleConns := maxConcurrent * 2
	if idleConns < 100 {
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

// Probe performs a GET against rawURL and reports whether the site is considered up.
// ctx cancels the probe; client performs the HTTP request (must be non-nil).
// rawURL is the absolute HTTP or HTTPS address to check.
func Probe(ctx context.Context, client *http.Client, rawURL string) Result {
	result := Result{URL: rawURL}
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		result.DurationMs = time.Since(start).Milliseconds()
		result.ErrMsg = err.Error()
		return result
	}
	SetBrowserLikeHeaders(req)

	resp, err := client.Do(req)
	result.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		result.ErrMsg = err.Error()
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	result.StatusCode = resp.StatusCode
	if !IsUpStatus(resp.StatusCode) {
		result.ErrMsg = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		return result
	}

	result.Up = true
	return result
}

// ProbeAll probes each URL concurrently and returns results in the same order as urls.
// ctx cancels outstanding probes; client performs HTTP requests.
// urls are absolute HTTP/HTTPS addresses to check.
// maxConcurrent caps simultaneous probes; values below 1 become 1.
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
		sem <- struct{}{}
		go func(idx int, u string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = Probe(ctx, client, u)
		}(i, rawURL)
	}

	wg.Wait()
	return results
}

// UnavailableURLs returns probe results that are not up, preserving input order.
// results are outcomes from Probe or ProbeAll.
func UnavailableURLs(results []Result) []Result {
	out := make([]Result, 0)
	for _, r := range results {
		if !r.Up {
			out = append(out, r)
		}
	}
	return out
}

package models

import (
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"
)

// MonitorCheckIntervalSeconds возвращает эффективный интервал проверки для монитора.
// monitor — отслеживаемый URL, у которого проверяется необязательное переопределение.
// globalSeconds — интервал по умолчанию из app_settings.
func MonitorCheckIntervalSeconds(monitor MonitorURL, globalSeconds int) int {
	if monitor.CheckIntervalSeconds != nil && *monitor.CheckIntervalSeconds >= 10 {
		// У монитора своё переопределение — минимум 10 секунд.
		return *monitor.CheckIntervalSeconds
	}
	return globalSeconds
}

// DefaultMonitorName возвращает имя хоста сайта из rawURL, если отображаемое имя не задано.
// rawURL — URL цели мониторинга, используемый для получения резервного имени.
func DefaultMonitorName(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		// Невалидный URL — показываем как есть после trim.
		return trimmed
	}
	return parsed.Host
}

// MonitorDisplayName возвращает настроенное имя или имя хоста URL по умолчанию.
// monitor — отслеживаемый URL, для которого определяется отображаемое имя.
func MonitorDisplayName(monitor MonitorURL) string {
	name := strings.TrimSpace(monitor.Name)
	if name != "" {
		return name
	}
	// Пустое имя — fallback на hostname из URL.
	return DefaultMonitorName(monitor.URL)
}

// ResolveMonitorName возвращает обрезанное name или значение по умолчанию, полученное из rawURL.
// name — необязательное отображаемое имя, заданное пользователем.
// rawURL используется, когда name пустое.
func ResolveMonitorName(name, rawURL string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	return DefaultMonitorName(rawURL)
}

// SeedMonitorURLs создаёт демонстрационные URL для мониторинга.
// db — дескриптор базы данных для вставки отсутствующих seed-строк.
// Возвращает число вновь созданных мониторов и любую ошибку персистентности.
func SeedMonitorURLs(db *gorm.DB) (int, error) {
	seeds := []MonitorURL{
		{Name: "Example.com", URL: "https://example.com"},
		{Name: "Example.org", URL: "https://www.example.org"},
		{Name: "HTTPBin 200", URL: "https://httpbin.org/status/200"},
		{Name: "Google Connectivity", URL: "https://www.google.com/generate_204"},
		{Name: "Cloudflare", URL: "https://www.cloudflare.com"},
		{Name: "Wikipedia", URL: "https://www.wikipedia.org"},
		{Name: "Go", URL: "https://go.dev"},
		{Name: "Python", URL: "https://www.python.org"},
		{Name: "Debian", URL: "https://www.debian.org"},
		{Name: "GNU", URL: "https://www.gnu.org"},
		{Name: "Linux Kernel", URL: "https://www.kernel.org"},
		{Name: "IETF", URL: "https://www.ietf.org"},
		{Name: "W3C", URL: "https://www.w3.org"},
		{Name: "OpenStreetMap", URL: "https://www.openstreetmap.org"},
		{Name: "DuckDuckGo", URL: "https://duckduckgo.com"},
		{Name: "Mozilla", URL: "https://www.mozilla.org"},
		{Name: "Internet Archive", URL: "https://archive.org"},
		{Name: "JSONPlaceholder", URL: "https://jsonplaceholder.typicode.com"},
		{Name: "RFC Editor", URL: "https://www.rfc-editor.org"},
		{Name: "Cloudflare Status", URL: "https://www.cloudflarestatus.com"},
	}
	created := 0
	for _, seed := range seeds {
		var existing MonitorURL
		// Идемпотентность: пропускаем URL, уже существующий в БД (unique по url).
		err := db.Where("url = ?", seed.URL).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, err
		}
		if err := db.Create(&seed).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

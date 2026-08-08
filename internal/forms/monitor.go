package forms

import (
	"fmt"
	"strconv"
	"strings"
)

// DTO форм создания и редактирования мониторов.
// Флаги уведомлений (NotifyTelegram, NotifySMTP) не биндятся из HTML автоматически — см. комментарии к полям.

// MonitorURLInput хранит данные формы создания или редактирования мониторимого URL.
type MonitorURLInput struct {
	Name                 string `form:"name" validate:"omitempty,max=200" label:"name"`
	URL                  string `form:"url" validate:"required,url,monitor_url" label:"url"`
	CheckIntervalSeconds string `form:"check_interval_seconds" label:"check interval"`
	// VerifyBeforeCreate при true выполняет живую HTTP-проверку URL до INSERT.
	// Если сайт недоступен, создание отклоняется — монитор не попадёт в БД «мёртвым».
	VerifyBeforeCreate bool `form:"verify_before_create"`
	// form:"-" — Gin ShouldBind игнорирует поле; значение выставляет handlers.bindMonitorNotificationFlags
	// после проверки, что канал настроен в Settings (чекбокс notify_telegram показывается только тогда).
	NotifyTelegram bool `form:"-"`
	// form:"-" — аналогично NotifyTelegram; handlers читает notify_smtp только если SMTPConfigured().
	NotifySMTP bool `form:"-"`
}

// ParseCheckIntervalSeconds преобразует необязательное поле формы в интервал конкретного монитора.
// Пустое значение означает, что монитор должен наследовать глобальную настройку.
func (input MonitorURLInput) ParseCheckIntervalSeconds() (*int, error) {
	raw := strings.TrimSpace(input.CheckIntervalSeconds)
	if raw == "" {
		// nil — наследовать глобальный интервал из app_settings.
		return nil, nil
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("check interval must be a whole number of seconds")
	}
	if seconds < 10 || seconds > 86400 {
		return nil, fmt.Errorf("check interval must be between 10 and 86400 seconds")
	}
	return &seconds, nil
}

// Validate проверяет MonitorURLInput и необязательное поле интервала проверки.
func (input MonitorURLInput) Validate() error {
	if err := validate.Struct(input); err != nil {
		return err
	}
	_, err := input.ParseCheckIntervalSeconds()
	return err
}

// MonitorURLBulkInput хранит данные формы массового создания мониторимых URL.
// Name не собирается: Name каждого монитора устанавливается равным его URL.
type MonitorURLBulkInput struct {
	URLs                 string `form:"urls"`
	CheckIntervalSeconds string `form:"check_interval_seconds" label:"check interval"`
	// VerifyBeforeCreate при true проверяет каждый URL живым HTTP-запросом до начала транзакции.
	// Режим «всё или ничего»: один недоступный URL отклоняет весь batch, ни один монитор не создаётся.
	VerifyBeforeCreate bool `form:"verify_before_create"`
	// SkipExisting при true молча пропускает URL, уже существующие в БД (unique по url).
	// При false дубликат прерывает batch с ошибкой конфликта — удобно для строгой проверки уникальности.
	SkipExisting bool `form:"skip_existing"`
	// form:"-" — см. MonitorURLInput; handlers.bindBulkMonitorNotificationFlags выставляет флаги вручную.
	NotifyTelegram bool `form:"-"`
	NotifySMTP     bool `form:"-"`
}

// ParseURLList разбивает raw на отдельные URL по запятым и переводам строк, обрезает пробелы,
// отбрасывает пустые записи и удаляет дубликаты, сохраняя порядок первого появления.
// raw — содержимое textarea, отправленное пользователем.
func ParseURLList(raw string) []string {
	// Нормализуем разделители: запятые и CRLF → одна строка на URL.
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", "\n")

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, part := range strings.Split(normalized, "\n") {
		url := strings.TrimSpace(part)
		if url == "" {
			continue
		}
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

// ParsedURLs возвращает дедуплицированный список URL из поля textarea.
func (input MonitorURLBulkInput) ParsedURLs() []string {
	return ParseURLList(input.URLs)
}

// ParseCheckIntervalSeconds преобразует необязательное поле формы в интервал конкретного монитора.
// Пустое значение означает, что каждый монитор должен наследовать глобальную настройку.
func (input MonitorURLBulkInput) ParseCheckIntervalSeconds() (*int, error) {
	return MonitorURLInput{CheckIntervalSeconds: input.CheckIntervalSeconds}.ParseCheckIntervalSeconds()
}

// Validate проверяет, что указан хотя бы один URL, каждый URL валиден для монитора,
// и необязательное поле интервала проверки корректно, если оно задано.
func (input MonitorURLBulkInput) Validate() error {
	urls := input.ParsedURLs()
	if len(urls) == 0 {
		return fmt.Errorf("at least one URL is required")
	}
	// Каждый URL валидируется отдельно — в ошибке указываем проблемный адрес.
	for _, rawURL := range urls {
		single := MonitorURLInput{URL: rawURL}
		if err := validate.Struct(single); err != nil {
			return fmt.Errorf("%s: %s", rawURL, FormatValidationError(err))
		}
	}
	_, err := input.ParseCheckIntervalSeconds()
	return err
}

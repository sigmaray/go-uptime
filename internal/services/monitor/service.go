// Package monitor содержит бизнес-логику создания и управления URL мониторов.
package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-uptime/internal/urlcheck"
	"go-uptime/models"

	"gorm.io/gorm"
)

// ErrMonitorURLExists — строка ошибки конфликта.
const ErrMonitorURLExists = "A monitor with this URL already exists"

const createVerifyConcurrency = 50

// Service предоставляет бизнес-логику для URL мониторов.
type Service struct {
	db *gorm.DB
}

// NewService создаёт новый сервис мониторов.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// URLExistsMessage формирует пользовательское сообщение о конфликте для одного или нескольких URL.
func URLExistsMessage(urls ...string) string {
	cleaned := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			cleaned = append(cleaned, u)
		}
	}
	if len(cleaned) == 0 {
		return ErrMonitorURLExists
	}
	// Перечисляем конфликтующие URL в сообщении для bulk-create.
	return fmt.Sprintf("%s: %s", ErrMonitorURLExists, strings.Join(cleaned, ", "))
}

// UnavailableMessage формирует пользовательскую ошибку, когда verify-before-create находит недоступные сайты.
func UnavailableMessage(failures []urlcheck.Result) string {
	if len(failures) == 0 {
		return "Site is unavailable and was not created"
	}
	parts := make([]string, 0, len(failures))
	for _, f := range failures {
		detail := strings.TrimSpace(f.ErrMsg)
		if detail == "" {
			parts = append(parts, f.URL)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", f.URL, detail))
	}
	if len(failures) == 1 {
		return fmt.Sprintf("Site is unavailable and was not created: %s", parts[0])
	}
	return fmt.Sprintf("Sites are unavailable and were not created: %s", strings.Join(parts, "; "))
}

// VerifyReachable проверяет urls по тем же правилам up/down, что и фоновый worker.
func VerifyReachable(ctx context.Context, urls []string) []urlcheck.Result {
	if len(urls) == 0 {
		return nil
	}
	client := urlcheck.NewClient(createVerifyConcurrency)
	results := urlcheck.ProbeAll(ctx, client, urls, createVerifyConcurrency)
	// Возвращаем только недоступные — caller решает, отклонять ли создание.
	return urlcheck.UnavailableURLs(results)
}

// ExistingURLs возвращает, какие из кандидатов уже сохранены как мониторы.
// Конфликты возвращаются в порядке candidates, а не в порядке строк из БД.
func (s *Service) ExistingURLs(candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	var found []string
	if err := s.db.Model(&models.MonitorURL{}).Where("url IN ?", candidates).Pluck("url", &found).Error; err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}

	existing := make(map[string]struct{}, len(found))
	for _, u := range found {
		existing[u] = struct{}{}
	}

	// Сохраняем порядок candidates — сообщение об ошибке повторяет порядок в форме.
	conflicts := make([]string, 0, len(found))
	for _, u := range candidates {
		if _, ok := existing[u]; ok {
			conflicts = append(conflicts, u)
		}
	}
	return conflicts, nil
}

// ExcludeURLs возвращает urls без записей, присутствующих в exclude.
func ExcludeURLs(urls, exclude []string) []string {
	if len(urls) == 0 || len(exclude) == 0 {
		return urls
	}

	skip := make(map[string]struct{}, len(exclude))
	for _, u := range exclude {
		skip[u] = struct{}{}
	}

	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if _, ok := skip[u]; ok {
			// URL уже в БД и SkipExisting=true — не создаём повторно.
			continue
		}
		out = append(out, u)
	}
	return out
}

// MarkUp переводит статус монитора в UP и закрывает открытый инцидент в одной транзакции.
// Возвращаемый wasDown == true означает «был переход DOWN→UP»; false, если IsUp был nil (ещё не проверяли).
func (s *Service) MarkUp(monitor models.MonitorURL, checkedAt time.Time, responseTimeMs *int) (bool, error) {
	var wasDown bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if monitor.IsUp != nil {
			wasDown = !(*monitor.IsUp)
		} else {
			// Первая проверка — не считаем переходом DOWN→UP для уведомлений.
			wasDown = false
		}

		updates := map[string]interface{}{
			"is_up":           true,
			"last_checked_at": checkedAt,
			"next_check_at":   checkedAt.Add(time.Duration(models.MonitorCheckIntervalSeconds(monitor, models.GetCheckIntervalSeconds(tx))) * time.Second),
			"last_error":      "",
		}
		if err := tx.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error; err != nil {
			return err
		}

		if err := models.RecordMonitorCheck(tx, monitor.ID, checkedAt, true, responseTimeMs); err != nil {
			return err
		}

		openIncident, err := models.FindOpenIncident(tx, monitor.ID)
		if err != nil {
			return err
		}
		if openIncident != nil {
			// Закрываем инцидент — resolved_at = время успешной проверки.
			if err := tx.Model(openIncident).Update("resolved_at", checkedAt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return wasDown, err
}

// MarkDown переводит статус монитора в DOWN и открывает/обновляет инцидент в одной транзакции.
// Возвращаемый wasUp == true означает «был переход UP→DOWN»; false, если IsUp был nil (ещё не проверяли).
func (s *Service) MarkDown(monitor models.MonitorURL, checkedAt time.Time, errMsg string, responseTimeMs *int) (bool, error) {
	var wasUp bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if monitor.IsUp != nil {
			wasUp = *monitor.IsUp
		} else {
			// Первая проверка сразу DOWN — не UP→DOWN для уведомлений.
			wasUp = false
		}

		updates := map[string]interface{}{
			"is_up":           false,
			"last_checked_at": checkedAt,
			"next_check_at":   checkedAt.Add(time.Duration(models.MonitorCheckIntervalSeconds(monitor, models.GetCheckIntervalSeconds(tx))) * time.Second),
			"last_error":      errMsg,
		}
		if err := tx.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error; err != nil {
			return err
		}

		if err := models.RecordMonitorCheck(tx, monitor.ID, checkedAt, false, responseTimeMs); err != nil {
			return err
		}

		openIncident, err := models.FindOpenIncident(tx, monitor.ID)
		if err != nil {
			return err
		}
		if openIncident == nil {
			// Новый простой — создаём открытый инцидент.
			incident := models.Incident{
				MonitorURLID: monitor.ID,
				StartedAt:    checkedAt,
				ErrorMessage: errMsg,
			}
			if err := tx.Create(&incident).Error; err != nil {
				return err
			}
		} else if openIncident.ErrorMessage != errMsg {
			// Инцидент уже открыт — обновляем текст ошибки при смене причины.
			if err := tx.Model(openIncident).Update("error_message", errMsg).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return wasUp, err
}

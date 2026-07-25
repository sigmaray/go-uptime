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

// ErrMonitorURLExists is the conflict error string.
const ErrMonitorURLExists = "A monitor with this URL already exists"

const createVerifyConcurrency = 10

// Service provides business logic for monitor URLs.
type Service struct {
	db *gorm.DB
}

// NewService creates a new monitor service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// URLExistsMessage builds a user-facing conflict message for one or more URLs.
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
	return fmt.Sprintf("%s: %s", ErrMonitorURLExists, strings.Join(cleaned, ", "))
}

// UnavailableMessage builds a user-facing error when verify-before-create finds unreachable sites.
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

// VerifyReachable probes urls with the same up/down rules as the background worker.
func VerifyReachable(ctx context.Context, urls []string) []urlcheck.Result {
	if len(urls) == 0 {
		return nil
	}
	client := urlcheck.NewClient(createVerifyConcurrency)
	results := urlcheck.ProbeAll(ctx, client, urls, createVerifyConcurrency)
	return urlcheck.UnavailableURLs(results)
}

// ExistingURLs returns which of the candidate URLs are already stored as monitors.
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

	conflicts := make([]string, 0, len(found))
	for _, u := range candidates {
		if _, ok := existing[u]; ok {
			conflicts = append(conflicts, u)
		}
	}
	return conflicts, nil
}

// ExcludeURLs returns urls with any entry present in exclude removed.
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
			continue
		}
		out = append(out, u)
	}
	return out
}

// MarkUp transitions the monitor status to UP and resolves any open incident in one transaction.
func (s *Service) MarkUp(monitor models.MonitorURL, checkedAt time.Time, responseTimeMs *int) (bool, error) {
	var wasDown bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if monitor.IsUp != nil {
			wasDown = !(*monitor.IsUp)
		} else {
			wasDown = false
		}

		updates := map[string]interface{}{
			"is_up":           true,
			"last_checked_at": checkedAt,
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
			if err := tx.Model(openIncident).Update("resolved_at", checkedAt).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return wasDown, err
}

// MarkDown transitions the monitor status to DOWN and opens/updates an incident in one transaction.
func (s *Service) MarkDown(monitor models.MonitorURL, checkedAt time.Time, errMsg string, responseTimeMs *int) (bool, error) {
	var wasUp bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if monitor.IsUp != nil {
			wasUp = *monitor.IsUp
		} else {
			wasUp = false
		}

		updates := map[string]interface{}{
			"is_up":           false,
			"last_checked_at": checkedAt,
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
			incident := models.Incident{
				MonitorURLID: monitor.ID,
				StartedAt:    checkedAt,
				ErrorMessage: errMsg,
			}
			if err := tx.Create(&incident).Error; err != nil {
				return err
			}
		} else if openIncident.ErrorMessage != errMsg {
			if err := tx.Model(openIncident).Update("error_message", errMsg).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return wasUp, err
}

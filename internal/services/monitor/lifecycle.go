package monitor

import (
	"errors"
	"time"

	"go-uptime/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SetActive pauses or resumes a monitor while preserving accurate uptime accounting.
// monitorID identifies the monitor whose scheduled checks are changed.
// active is true to resume checks and false to pause them.
func (s *Service) SetActive(monitorID uint, active bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var monitor models.MonitorURL
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&monitor, monitorID).Error; err != nil {
			return err
		}
		if monitor.Active == active {
			return nil
		}

		return setMonitorActive(tx, monitor, active, time.Now().UTC())
	})
}

// SetAllActive changes the active state of every monitor in one transaction.
// active is true to resume all monitors and false to pause them.
// It returns how many monitor states changed.
func (s *Service) SetAllActive(active bool) (int, error) {
	changed := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var monitors []models.MonitorURL
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("active <> ?", active).
			Order("id asc").
			Find(&monitors).Error; err != nil {
			return err
		}

		if !active {
			now := time.Now().UTC()
			intervals := make([]models.UptimeInterval, 0, len(monitors))
			for _, monitor := range monitors {
				firstCheckAfterResume := monitor.LastCheckedAt != nil && monitor.NextCheckAt != nil && !monitor.NextCheckAt.After(*monitor.LastCheckedAt)
				if firstCheckAfterResume || monitor.IsUp == nil || monitor.LastCheckedAt == nil {
					continue
				}
				intervals = append(intervals, models.UptimeInterval{
					MonitorURLID: monitor.ID,
					StartedAt:    *monitor.LastCheckedAt,
					EndedAt:      now,
					IsUp:         *monitor.IsUp,
				})
			}
			if err := models.ApplyUptimeIntervals(tx, intervals); err != nil {
				return err
			}
		}

		updates := map[string]interface{}{"active": active}
		if active {
			// Use each monitor's last observed check as a one-shot marker for resume.
			updates["next_check_at"] = gorm.Expr("last_checked_at")
		} else {
			updates["next_check_at"] = nil
		}
		result := tx.Model(&models.MonitorURL{}).Where("active <> ?", active).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		changed = int(result.RowsAffected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

// setMonitorActive updates one already locked monitor and its uptime boundary.
// tx is the transaction that holds a row lock for monitor.
// monitor is the current persisted monitor state.
// active is true to resume checks and false to pause them.
// now is the UTC timestamp used for this lifecycle transition.
func setMonitorActive(tx *gorm.DB, monitor models.MonitorURL, active bool, now time.Time) error {
	updates := map[string]interface{}{"active": active}
	if active {
		// Reuse the due schedule as a one-shot marker so the first resumed result
		// does not count the paused interval as uptime or downtime.
		updates["next_check_at"] = monitor.LastCheckedAt
	} else {
		updates["next_check_at"] = nil
		// Close the last observed active interval at pause time using the last
		// observed state, so active time before pausing is retained.
		firstCheckAfterResume := monitor.LastCheckedAt != nil && monitor.NextCheckAt != nil && !monitor.NextCheckAt.After(*monitor.LastCheckedAt)
		if !firstCheckAfterResume && monitor.IsUp != nil && monitor.LastCheckedAt != nil {
			if err := models.ApplyUptimeInterval(tx, monitor.ID, monitor.LastCheckedAt, now, *monitor.IsUp); err != nil {
				return err
			}
		}
	}

	return tx.Model(&models.MonitorURL{}).Where("id = ?", monitor.ID).Updates(updates).Error
}

// IsNotFound reports whether SetActive failed because the monitor does not exist.
// err is the error returned by SetActive.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

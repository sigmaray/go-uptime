package cli

import (
	"fmt"

	monitorsvc "go-uptime/internal/services/monitor"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MonitorsPauseAll pauses every monitor while preserving uptime accounting.
// db is the database handle connected to the application database.
func MonitorsPauseAll(db *gorm.DB) {
	setAllMonitorsActive(db, false)
}

// MonitorsResumeAll resumes every monitor while preserving uptime accounting.
// db is the database handle connected to the application database.
func MonitorsResumeAll(db *gorm.DB) {
	setAllMonitorsActive(db, true)
}

// setAllMonitorsActive changes every monitor's scheduled-check state.
// db is the database handle connected to the application database.
// active is true to resume all monitors and false to pause them.
func setAllMonitorsActive(db *gorm.DB, active bool) {
	changed, err := monitorsvc.NewService(db).SetAllActive(active)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to change all monitor active states")
	}

	if active {
		fmt.Printf("Resumed all monitors (%d changed).\n", changed)
		return
	}
	fmt.Printf("Paused all monitors (%d changed).\n", changed)
}

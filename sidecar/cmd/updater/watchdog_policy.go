package main

import (
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

// watchdogTickInterval gives each deadline one sampling interval of grace.
const watchdogTickInterval = 30 * time.Second

// phaseDefaultDeadline covers phases whose worker has not reported a sub-step.
const phaseDefaultDeadline = 30 * time.Minute

// subStepDeadlines bounds each update sub-step.
var subStepDeadlines = map[state.SubStep]time.Duration{
	state.SubStepResolving:   60 * time.Second,
	state.SubStepPreflight:   60 * time.Second,
	state.SubStepStopping:    60 * time.Second,
	state.SubStepBackup:      120 * time.Second,
	state.SubStepDownloading: 900 * time.Second,
	state.SubStepVerifying:   60 * time.Second,
	state.SubStepLoading:     120 * time.Second,
	state.SubStepStarting:    120 * time.Second,
	state.SubStepMigrating:   600 * time.Second,
	state.SubStepHealthCheck: 180 * time.Second,
	state.SubStepReporting:   120 * time.Second,
	state.SubStepSpawning:    30 * time.Second,
}

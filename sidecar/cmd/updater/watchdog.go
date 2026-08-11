package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

// runWatchdog marks any operation that exceeds its current deadline as failed.
func runWatchdog(ctx context.Context, dataDir string, states []*state.State, upd *updater.Deps) {
	ticker := time.NewTicker(watchdogTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, s := range states {
				checkStuck(s, dataDir, upd)
			}
		}
	}
}

// checkStuck evaluates one state snapshot and preserves failure artifacts when
// its current phase or sub-step exceeds the configured deadline.
func checkStuck(s *state.State, dataDir string, upd *updater.Deps) {
	snap := s.Snapshot()
	switch snap.Phase {
	case state.Idle, state.Error, state.Staged:
		return
	}
	deadline := phaseDefaultDeadline
	if dl, ok := subStepDeadlines[snap.SubStep]; ok {
		deadline = dl
	}
	enteredAt := s.PhaseEnteredAt()
	stuck := time.Since(enteredAt)
	if stuck <= deadline {
		return
	}
	log.Printf("watchdog: kind=%s phase=%s sub_step=%q stuck for %s (deadline=%s) — marking Error",
		s.Kind(), snap.Phase, snap.SubStep, stuck.Round(time.Second), deadline)
	s.MarkError(
		"WATCHDOG_TIMEOUT",
		fmt.Sprintf("phase %s (sub_step=%s) exceeded deadline %s — sidecar restart or /force-reset required",
			snap.Phase, snap.SubStep, deadline),
	)
	// Record the timeout before cancellation so a late worker cannot hide it.
	if upd != nil {
		if upd.Cancel() {
			log.Printf("watchdog: cancelled in-flight worker for kind=%s", s.Kind())
		}
	}
	if snap.UpdateID != "" {
		updater.ArchiveUpdateLogToFailures(dataDir, snap.UpdateID, "watchdog_timeout", nil)
	}
}

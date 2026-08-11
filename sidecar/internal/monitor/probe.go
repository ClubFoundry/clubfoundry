package monitor

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
)

func (m *Monitor) probe(ctx context.Context) {
	// Updates and rollbacks intentionally make the main app unavailable.
	if snap := m.State.Snapshot(); snap.Phase == state.Updating || snap.Phase == state.RollingBack {
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	ok, _, _ := m.Checker.Probe(probeCtx)
	if ok {
		m.mu.Lock()
		m.consecFails = 0
		m.consecOks++
		oks := m.consecOks
		m.mu.Unlock()

		m.maybeAutoSoftRecover(ctx, oks)
		return
	}

	m.mu.Lock()
	m.consecFails++
	// A failed probe restarts the sustained-health recovery window.
	m.consecOks = 0
	failed := m.consecFails
	halted := !m.haltedAt.IsZero()
	m.mu.Unlock()

	if halted {
		return
	}
	if failed < m.FailThreshold {
		log.Printf("monitor: main-app probe failed (%d/%d)", failed, m.FailThreshold)
		return
	}

	if m.tooManyRecentUpAttempts() {
		if m.tryAutoRollback(ctx) {
			m.mu.Lock()
			m.upAttempts = nil
			m.consecFails = 0
			m.reinstallTriedAt = time.Time{}
			m.mu.Unlock()
			return
		}
		if m.tryAutoReinstall(ctx) {
			m.mu.Lock()
			m.upAttempts = nil
			m.consecFails = 0
			m.reinstallTriedAt = time.Time{}
			m.mu.Unlock()
			return
		}
		m.halt(ctx, "crash-loop: rollback skipped/failed, reinstall skipped/failed")
		return
	}

	log.Printf("monitor: main-app unresponsive for %d probes — docker compose up -d %s",
		failed, m.Docker.MainService)
	// Count the attempt before Up so a slow failure cannot bypass escalation.
	m.recordUpAttempt()
	restartCtx, cancel2 := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel2()
	err := m.Docker.Up(restartCtx, m.Docker.MainService)
	m.mu.Lock()
	m.consecFails = 0
	m.mu.Unlock()
	if err != nil {
		log.Printf("monitor: docker compose up FAILED: %v", err)
		return
	}
	log.Printf("monitor: main-app recovered via docker compose up (after %d failed probes)", failed)
	m.recordEvent(recoveryhistory.KindRecover,
		"main app unresponsive on /health — recovered by recreating the container from the local image")
}

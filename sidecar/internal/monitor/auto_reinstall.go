package monitor

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/recoveryhistory"
)

// tryAutoReinstall allows one same-version reinstall per halt cycle.
func (m *Monitor) tryAutoReinstall(ctx context.Context) bool {
	if m.Reinstall == nil {
		return false
	}
	m.mu.Lock()
	already := !m.reinstallTriedAt.IsZero()
	if !already {
		m.reinstallTriedAt = time.Now()
	}
	m.mu.Unlock()
	if already {
		log.Printf("monitor: reinstall already attempted this halt cycle — halt path")
		return false
	}
	log.Printf("monitor: crash-loop persists, attempting same-version reinstall (Greenboot #6)")
	rinCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if err := m.Reinstall.RunReinstallCurrent(rinCtx); err != nil {
		log.Printf("monitor: auto-reinstall FAILED: %v — falling back to halt", err)
		return false
	}
	log.Printf("monitor: auto-reinstall succeeded; clearing restart tally")
	m.recordEvent(recoveryhistory.KindReinstall,
		"repeated recovery attempts failed — reinstalled the same version from the cloud and recreated the container")
	if m.Cloud != nil && m.Cloud.BaseURL != "" {
		go func() {
			alertCtx, alertCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer alertCancel()
			_ = m.Cloud.PostAlert(alertCtx, "auto-reinstall fired after rollback skipped/failed")
		}()
	}
	return true
}

package monitor

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/recoveryhistory"
)

// tryAutoRollback rolls back only failures close to a successful update.
func (m *Monitor) tryAutoRollback(ctx context.Context) bool {
	if m.Rollback == nil {
		return false
	}
	last := m.Rollback.LastSuccessfulMainUpdate()
	if last.IsZero() {
		log.Printf("monitor: crash-loop but no recorded successful main update — halt path")
		return false
	}
	since := time.Since(last)
	if since > m.PostUpdateRollbackWindow {
		log.Printf("monitor: crash-loop %s after last update (window=%s) — runtime crash, halt path",
			since.Round(time.Second), m.PostUpdateRollbackWindow)
		return false
	}
	log.Printf("monitor: crash-loop %s after last successful update — auto-rollback to previous image",
		since.Round(time.Second))
	rbCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if err := m.Rollback.RunRollback(rbCtx); err != nil {
		log.Printf("monitor: auto-rollback FAILED: %v — falling back to halt", err)
		return false
	}
	log.Printf("monitor: auto-rollback succeeded; clearing restart tally")
	m.recordEvent(recoveryhistory.KindReinstall,
		"crash-loop within post-update window — rolled back to the previous version")
	if m.Cloud != nil && m.Cloud.BaseURL != "" {
		go func() {
			alertCtx, alertCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer alertCancel()
			_ = m.Cloud.PostAlert(alertCtx, "auto-rollback fired after crash-loop within post-update window")
		}()
	}
	return true
}

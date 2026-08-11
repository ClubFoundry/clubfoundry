package monitor

import (
	"context"
	"log"

	"github.com/clubfoundry/updater/internal/recoveryhistory"
)

// maybeAutoSoftRecover clears stale errors after their health gate is met.
func (m *Monitor) maybeAutoSoftRecover(ctx context.Context, consecOks int) {
	if m.AutoSoftRecoverWindow <= 0 {
		return
	}
	required := 1
	if m.ProbeInterval > 0 {
		required = int((m.AutoSoftRecoverWindow + m.ProbeInterval - 1) / m.ProbeInterval)
		if required < 1 {
			required = 1
		}
	}
	if m.State != nil && consecOks >= required {
		if m.State.AutoSoftRecoverIfStuck(m.AutoSoftRecoverWindow) {
			log.Printf("monitor: main-state auto-soft-recovered from Error after %s of /health=ok", m.AutoSoftRecoverWindow)
			m.recordEvent(recoveryhistory.KindRecover,
				"main-state auto-soft-recovered — error cleared after sustained /health=ok window")
		}
	}
	if m.SelfState != nil {
		if m.SelfState.AutoSoftRecoverIfStuck(m.AutoSoftRecoverWindow) {
			log.Printf("monitor: self-state auto-soft-recovered from Error after %s in Error", m.AutoSoftRecoverWindow)
			m.recordEvent(recoveryhistory.KindRecover,
				"self-state auto-soft-recovered — stale self-update error cleared")
		}
	}
	_ = ctx
}

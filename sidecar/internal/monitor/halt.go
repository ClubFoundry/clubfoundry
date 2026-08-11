package monitor

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/recoveryhistory"
)

func (m *Monitor) halt(ctx context.Context, reason string) {
	m.mu.Lock()
	already := !m.haltedAt.IsZero()
	if !already {
		m.haltedAt = time.Now()
	}
	m.mu.Unlock()
	if already {
		return
	}
	log.Printf("monitor: HALT — %s", reason)
	m.State.MarkError("CRASH_LOOP", reason)
	m.recordEvent(recoveryhistory.KindHalt, reason)
	m.sendAlert(ctx, reason)
}

// sendAlert reports a halt asynchronously when cloud access is configured.
func (m *Monitor) sendAlert(ctx context.Context, reason string) {
	if m.Cloud == nil || m.Cloud.BaseURL == "" {
		return
	}
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = m.Cloud.PostAlert(sendCtx, reason)
	}()
}

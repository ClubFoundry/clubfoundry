package monitor

import (
	"context"
	"time"
)

// Run probes until the caller cancels the context.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.ProbeInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.probe(ctx)
		}
	}
}

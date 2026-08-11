package health

import (
	"context"
	"fmt"
	"time"
)

// ProgressFn receives non-empty reports while WaitHealthy is polling.
type ProgressFn func(r Report)

// WaitHealthy polls until the app reports healthy or the context ends.
func (c *Checker) WaitHealthy(ctx context.Context, interval time.Duration, onProgress ...ProgressFn) (Report, error) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	emit := func(r Report) {
		for _, fn := range onProgress {
			if fn != nil {
				fn(r)
			}
		}
	}

	if ok, r, _ := c.Probe(ctx); ok {
		emit(r)
		return r, nil
	} else if r.Status != "" {
		emit(r)
	}
	for {
		select {
		case <-ctx.Done():
			return Report{}, fmt.Errorf("wait healthy: %w", ctx.Err())
		case <-ticker.C:
			if ok, r, _ := c.Probe(ctx); ok {
				emit(r)
				return r, nil
			} else if r.Status != "" {
				emit(r)
			}
		}
	}
}

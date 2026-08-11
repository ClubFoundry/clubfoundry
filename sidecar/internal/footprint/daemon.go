package footprint

import (
	"context"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// StartDaemon reads fresh settings for each scheduled prune cycle.
func StartDaemon(ctx context.Context, dock dockerops.Config, getCfg func() PruneConfig, firstAfter, period time.Duration, logf func(format string, args ...any)) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	go func() {
		select {
		case <-time.After(firstAfter):
		case <-ctx.Done():
			return
		}
		runAndLog(ctx, dock, getCfg, logf)
		t := time.NewTicker(period)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runAndLog(ctx, dock, getCfg, logf)
			}
		}
	}()
}

func runAndLog(ctx context.Context, dock dockerops.Config, getCfg func() PruneConfig, logf func(format string, args ...any)) {
	cfg := getCfg()
	cfg.Now = time.Now()
	out, err := RunOnce(ctx, dock, cfg)
	if err != nil {
		logf("auto-prune: cycle failed: %v", err)
		return
	}
	removed, kept, errors := summarize(out)
	logf("auto-prune: cycle complete (removed=%d kept=%d errors=%d)", removed, kept, errors)
}

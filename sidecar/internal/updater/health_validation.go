package updater

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// smokeTest re-probes health after initial acceptance to catch delayed startup
// failures and version drift. Any error triggers the caller's rollback path.
const smokeTestDelay = 5 * time.Second

func (d *Deps) smokeTest(ctx context.Context, expectedVersion string, logW io.Writer) error {
	fmt.Fprintf(logW, "[smoke] re-probing /health after %s\n", smokeTestDelay)
	// Initial health already accepted the update. Keep this delay
	// non-cancellable so a late cancel cannot turn a completed update into a
	// misleading cancelled rollback; the probe itself remains bounded by ctx.
	time.Sleep(smokeTestDelay)
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	healthy, report, err := d.Health.Probe(probeCtx)
	if err != nil {
		return fmt.Errorf("re-probe network error: %w", err)
	}
	if !healthy {
		return fmt.Errorf("re-probe unhealthy: status=%q ready=%v phase=%q", report.Status, report.Ready, report.Phase)
	}
	if expectedVersion != "" && report.Version != "" && report.Version != expectedVersion {
		return fmt.Errorf("version drift between consecutive probes: first=%q second=%q (image swap or compose race?)",
			expectedVersion, report.Version)
	}
	fmt.Fprintf(logW, "[smoke] /health still ok: version=%q\n", report.Version)
	return nil
}

// expectedVersion returns a concrete version tag suitable for comparison with
// /health. Floating aliases, digest-only refs, and untagged image refs skip the
// comparison.
func expectedVersion(target string) string {
	switch target {
	case "", "latest", "unknown", "previous", "current":
		return ""
	}
	if tag := extractTagFromImageRef(target); tag != "" {
		return tag
	}
	if strings.ContainsAny(target, "/@") {
		return ""
	}
	return target
}

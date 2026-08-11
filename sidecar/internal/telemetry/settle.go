package telemetry

import (
	"context"
	"time"
)

// checkSettleHealth classifies app health after the settling window.
func (r *Reporter) checkSettleHealth(ctx context.Context, expectedVersion string) string {
	if r.Health == nil {
		return "unknown"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ok, rep, err := r.Health.Probe(probeCtx)
	if err != nil || !ok {
		return "unhealthy"
	}
	if expectedVersion != "" && expectedVersion != "latest" && expectedVersion != "unknown" &&
		rep.Version != "" && rep.Version != expectedVersion {
		return "version_mismatch"
	}
	return "healthy"
}

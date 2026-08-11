// Package telemetry reports update outcomes after a settling delay. Reports
// include versions, duration, diagnostics, and post-update health. Delivery is
// best-effort; local diagnostics remain available when the cloud is offline.
package telemetry

import (
	"context"

	"github.com/clubfoundry/updater/internal/history"
)

// FireAfterSettle asynchronously verifies health and reports the completed
// update. It does not inherit the caller deadline because update request
// contexts normally end before the settling window.
func (r *Reporter) FireAfterSettle(_ context.Context, entry history.Entry) {
	if r.CloudBaseURL == "" || r.InstanceID == "" {
		return // telemetry disabled (air-gapped install)
	}
	go r.runReport(entry)
}

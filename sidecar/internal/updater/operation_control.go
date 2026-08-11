package updater

import (
	"context"
	"errors"
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

// armCancel exposes the active operation to the cancellation endpoint.
func (d *Deps) armCancel(c context.CancelFunc) {
	d.cancelMu.Lock()
	d.cancelFn = c
	d.cancelMu.Unlock()
}

func (d *Deps) disarmCancel() {
	d.cancelMu.Lock()
	d.cancelFn = nil
	d.cancelMu.Unlock()
}

// Cancel signals the active operation and reports whether one was armed.
// The operation remains armed until its deferred disarmCancel runs.
func (d *Deps) Cancel() bool {
	d.cancelMu.Lock()
	fn := d.cancelFn
	d.cancelMu.Unlock()
	if fn == nil {
		return false
	}
	d.State.RequestCancel()
	fn()
	return true
}

// errCancelled distinguishes an operator request from an operational failure.
var errCancelled = errors.New("operation cancelled by operator")

// isCancelled requires an inner cancellation while the parent remains active.
func isCancelled(err error, parent, inner context.Context) bool {
	if err == nil {
		return false
	}
	innerDone := inner.Err() != nil
	parentDone := parent.Err() != nil
	if !innerDone {
		return false
	}
	if !parentDone {
		return true
	}
	return false
}

func (d *Deps) effectiveStartup() time.Duration {
	if d.StartupWindow > 0 {
		return d.StartupWindow
	}
	return 60 * time.Second
}

func (d *Deps) logDir() string {
	if d.LogDir != "" {
		return d.LogDir
	}
	return "/app/data/update-logs"
}

// openUpdateLog applies retention before creating a nil-safe operation log.
// Legacy flat logs are migrated by the lower-level opener.
func (d *Deps) openUpdateLog(updateID, opID string, kind state.Kind) *updateLog {
	SweepDiagnosticRetention(d.DataDir)
	return openUpdateLog(d.logDir(), updateID, opID, kind)
}

package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

// Apply installs an image prepared by Stage without pulling it again. Failures
// use the same backup and rollback path as a regular update.
func (d *Deps) Apply(parent context.Context) error {
	staged := d.State.StagedTarget()
	if staged == "" {
		return fmt.Errorf("nothing staged: call /update/stage first")
	}

	started := time.Now()
	updateID := fmt.Sprintf("upd-%d", started.UnixNano())
	opID := newOpID()
	ulog := d.openUpdateLog(updateID, opID, state.KindMain)
	defer func() {
		if d.State != nil {
			d.State.RegisterChangeHook(nil)
			ulog.writeStateSnapshot("state-post", d.State.Snapshot())
		}
		ulog.close()
	}()
	if d.State != nil {
		ulog.writeStateSnapshot("state-pre", d.State.Snapshot())
		d.State.RegisterChangeHook(ulog.hookFn())
	}
	logW := ulog.writer()

	fmt.Fprintf(logW, "=== Apply %s started %s (target=%s) op_id=%s ===\n", updateID, started.Format(time.RFC3339), staged, opID)
	if err := d.State.TransitionTo(state.Updating, fmt.Sprintf("applying staged %s", staged)); err != nil {
		return err
	}
	d.State.SetOpID(opID)
	d.State.SetUpdateID(updateID)
	d.State.SetTarget(staged)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d.armCancel(cancel)
	defer d.disarmCancel()

	fromVersion := d.CurrentVersion(ctx)
	backupPath, runErr := d.doApply(ctx, fromVersion, staged, logW)
	finished := time.Now()
	entry := history.Entry{
		ID:          updateID,
		StartedAt:   started,
		FinishedAt:  finished,
		DurationMS:  finished.Sub(started).Milliseconds(),
		FromVersion: fromVersion,
		ToVersion:   staged,
	}

	if runErr == nil {
		entry.Outcome = history.OutcomeSuccess
		_ = d.Backup.PruneOld()
		d.appendHistory(entry)
		fmt.Fprintf(logW, "=== Apply %s SUCCESS (%dms) ===\n", updateID, entry.DurationMS)
		_ = d.State.TransitionTo(state.Idle, "")
		if d.Telemetry != nil {
			d.Telemetry.FireAfterSettle(parent, entry)
		}
		return nil
	}

	cancelled := isCancelled(runErr, parent, ctx)
	if cancelled {
		fmt.Fprintf(logW, "apply cancelled by operator — running rollback path\n")
	} else {
		fmt.Fprintf(logW, "apply failed: %v — attempting rollback\n", runErr)
	}
	rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer rollbackCancel()
	rollbackErr := d.doRollback(rollbackCtx, backupPath, fromVersion, logW)
	entry.Error = runErr.Error()

	if rollbackErr != nil {
		entry.Outcome = history.OutcomeError
		entry.Error = fmt.Sprintf("apply failed: %v; rollback also failed: %v", runErr, rollbackErr)
		d.appendHistory(entry)
		d.State.MarkError(classifyError(runErr, rollbackErr), entry.Error)
		fmt.Fprintf(logW, "=== Apply %s FAILED %s ===\n", updateID, entry.Error)
		ArchiveUpdateLogToFailures(d.DataDir, updateID, "apply_rollback_failed", logW)
		return fmt.Errorf("%s", entry.Error)
	}
	if cancelled {
		entry.Outcome = history.OutcomeCancelled
		entry.Error = ""
	} else {
		entry.Outcome = history.OutcomeRollback
	}
	d.appendHistory(entry)
	if cancelled {
		fmt.Fprintf(logW, "=== Apply %s CANCELLED — rolled back to %s ===\n", updateID, fromVersion)
	} else {
		fmt.Fprintf(logW, "=== Apply %s ROLLED BACK OK ===\n", updateID)
		d.State.MarkError(classifyError(runErr, nil), entry.Error)
		d.State.ClearError()
		ArchiveUpdateLogToFailures(d.DataDir, updateID, "apply_rollback", logW)
	}
	_ = d.State.TransitionTo(state.Idle, "")
	if d.Telemetry != nil {
		d.Telemetry.FireAfterSettle(parent, entry)
	}
	if cancelled {
		return errCancelled
	}
	return fmt.Errorf("apply failed and rolled back: %w", runErr)
}

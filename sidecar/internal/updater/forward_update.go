package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

// RunUpdate performs one main-application update. A configured URL uses a
// verified tarball; an empty URL delegates the pull to Docker Compose.
func (d *Deps) RunUpdate(parent context.Context, target string, opts dockerops.PullOpts) error {
	started := time.Now()
	updateID := fmt.Sprintf("upd-%d", started.UnixNano())
	opID := newOpID()

	// Scope the state hook to this operation so later administrative changes do
	// not appear in its diagnostic log.
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

	fmt.Fprintf(logW, "=== Update %s started %s ===\n", updateID, started.Format(time.RFC3339))
	fmt.Fprintf(logW, "target=%q artifact_url=%q op_id=%q\n", target, opts.URL, opID)

	if err := d.State.TransitionTo(state.Updating, fmt.Sprintf("updating to %s", orLatest(target))); err != nil {
		return err // already busy
	}
	d.State.SetOpID(opID)
	d.State.SetUpdateID(updateID)
	d.State.SetTarget(orLatest(target))

	// Publish the child cancel function only while this operation is active.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d.armCancel(cancel)
	defer d.disarmCancel()

	fromVersion := d.CurrentVersion(ctx)
	toVersion := orLatest(target)
	fmt.Fprintf(logW, "from=%q to=%q\n", fromVersion, toVersion)

	// Mirror download progress into the persisted operation state.
	opts.LogWriter = logW
	opts.ProgressFn = func(downloaded, total int64, bps float64) {
		var eta int64
		if bps > 0 && total > downloaded && total > 0 {
			eta = int64(float64(total-downloaded) / bps)
		}
		d.State.UpdateDownload(state.DownloadProgress{
			BytesDownloaded: downloaded,
			BytesTotal:      total,
			BytesPerSecond:  bps,
			ETASeconds:      eta,
		})
	}
	// Loading is distinct from downloading so a completed progress bar never
	// appears stalled while Docker imports the image.
	opts.OnLoadStart = func() {
		d.State.UpdateSubStep(state.SubStepLoading, "Unpacking and loading image")
	}

	backupPath, runErr := d.doUpdate(ctx, fromVersion, toVersion, opts, logW)
	finished := time.Now()

	entry := history.Entry{
		ID:          updateID,
		StartedAt:   started,
		FinishedAt:  finished,
		DurationMS:  finished.Sub(started).Milliseconds(),
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}

	if runErr == nil {
		entry.Outcome = history.OutcomeSuccess
		_ = d.Backup.PruneOld()
		// Persist success before advancing retention tags. Recovery decisions use
		// history as the durable source of the last successful update.
		d.appendHistory(entry)
		// Retention tags are best-effort because the versioned tags remain the
		// primary rollback references.
		d.tagRetention(ctx, fromVersion, toVersion, logW)
		fmt.Fprintf(logW, "=== Update %s SUCCESS (%dms) ===\n", updateID, entry.DurationMS)
		_ = d.State.TransitionTo(state.Idle, "")
		if d.Telemetry != nil {
			d.Telemetry.FireAfterSettle(ctx, entry)
		}
		return nil
	}

	// Cancellation uses the same recovery path as failure because destructive
	// steps may already have started. History retains a distinct outcome.
	cancelled := isCancelled(runErr, parent, ctx)
	if cancelled {
		fmt.Fprintf(logW, "update cancelled by operator — running rollback path\n")
	} else {
		fmt.Fprintf(logW, "update failed: %v — attempting rollback\n", runErr)
	}
	// Recovery gets a fresh bounded context so cancellation cannot interrupt it.
	rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer rollbackCancel()
	rollbackErr := d.doRollback(rollbackCtx, backupPath, fromVersion, logW)
	entry.Error = runErr.Error()
	if rollbackErr != nil {
		entry.Outcome = history.OutcomeError
		entry.Error = fmt.Sprintf("update failed: %v; rollback also failed: %v", runErr, rollbackErr)
		d.appendHistory(entry)
		d.State.MarkError(classifyError(runErr, rollbackErr), entry.Error)
		fmt.Fprintf(logW, "=== Update %s FAILED %s ===\n", updateID, entry.Error)
		// Preserve both the machine-readable failure and its operation artifacts.
		writeFailureBundle(d.DataDir, entry, classifyError(runErr, rollbackErr),
			"rollback", ulog.logFilePath(), logW)
		ArchiveUpdateLogToFailures(d.DataDir, updateID, "rollback_failed", logW)
		return fmt.Errorf("%s", entry.Error)
	}
	if cancelled {
		entry.Outcome = history.OutcomeCancelled
		entry.Error = "" // The operator intentionally aborted the operation.
	} else {
		entry.Outcome = history.OutcomeRollback
	}
	d.appendHistory(entry)
	if cancelled {
		fmt.Fprintf(logW, "=== Update %s CANCELLED — rolled back to %s ===\n", updateID, fromVersion)
	} else {
		fmt.Fprintf(logW, "=== Update %s ROLLED BACK OK ===\n", updateID)
		// Keep the error visible in state so UI surfaces the rollback banner.
		d.State.MarkError(classifyError(runErr, nil), entry.Error)
		d.State.ClearError() // Error was informational; rollback succeeded.
		// A recovered failure still gets a local bundle for later diagnosis.
		writeFailureBundle(d.DataDir, entry, classifyError(runErr, nil),
			"rollback", ulog.logFilePath(), logW)
		ArchiveUpdateLogToFailures(d.DataDir, updateID, "rollback", logW)
	}
	_ = d.State.TransitionTo(state.Idle, "")
	if d.Telemetry != nil {
		d.Telemetry.FireAfterSettle(parent, entry)
	}
	if cancelled {
		return errCancelled
	}
	return fmt.Errorf("update failed and rolled back: %w", runErr)
}

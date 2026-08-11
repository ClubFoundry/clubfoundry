package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

// Stage downloads and verifies an image without stopping the running service.
// Apply performs the destructive half later in the maintenance window.
func (d *Deps) Stage(parent context.Context, target string, opts dockerops.PullOpts) error {
	started := time.Now()
	updateID := fmt.Sprintf("stg-%d", started.UnixNano())
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

	fmt.Fprintf(logW, "=== Stage %s started %s ===\n", updateID, started.Format(time.RFC3339))
	fmt.Fprintf(logW, "target=%q artifact_url=%q op_id=%q\n", target, opts.URL, opID)

	if err := d.State.TransitionTo(state.Staging, fmt.Sprintf("staging %s", orLatest(target))); err != nil {
		return err
	}
	d.State.SetOpID(opID)
	d.State.SetUpdateID(updateID)
	d.State.SetTarget(orLatest(target))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d.armCancel(cancel)
	defer d.disarmCancel()

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

	fromVersion := d.CurrentVersion(ctx)
	toVersion := orLatest(target)
	stageErr := d.doStage(ctx, fromVersion, toVersion, opts, logW)
	finished := time.Now()
	entry := history.Entry{
		ID:          updateID,
		StartedAt:   started,
		FinishedAt:  finished,
		DurationMS:  finished.Sub(started).Milliseconds(),
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}

	if stageErr == nil {
		// A successful Stage is only half an update. Apply writes the completed
		// history entry after the image is actually running.
		_ = d.State.TransitionTo(state.Staged, fmt.Sprintf("Staged %s — ready to install", toVersion))
		d.State.SetStagedTarget(toVersion)
		fmt.Fprintf(logW, "=== Stage %s OK — image cached, ready for /update/apply ===\n", updateID)
		return nil
	}

	if isCancelled(stageErr, parent, ctx) {
		entry.Outcome = history.OutcomeCancelled
		d.appendHistory(entry)
		fmt.Fprintf(logW, "=== Stage %s CANCELLED ===\n", updateID)
		_ = d.State.TransitionTo(state.Idle, "")
		return errCancelled
	}

	entry.Outcome = history.OutcomeError
	entry.Error = stageErr.Error()
	d.appendHistory(entry)
	fmt.Fprintf(logW, "=== Stage %s FAILED: %v ===\n", updateID, stageErr)
	d.State.MarkError(classifyError(stageErr, nil), stageErr.Error())
	ArchiveUpdateLogToFailures(d.DataDir, updateID, "stage_failed", logW)
	return stageErr
}

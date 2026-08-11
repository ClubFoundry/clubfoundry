package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/history"
)

func (d *Deps) pullSelfImage(ctx, parent context.Context, targetVersion string, opts dockerops.PullOpts, op *selfUpdateOperation) error {
	err := d.Docker.Pull(ctx, d.Docker.UpdaterServiceName(), targetVersion, opts)
	if err == nil {
		return nil
	}

	entry := history.Entry{
		ID:          op.updateID,
		StartedAt:   op.started,
		FinishedAt:  time.Now(),
		FromVersion: op.fromVersion,
		ToVersion:   op.toVersion,
		Outcome:     history.OutcomeError,
		Error:       err.Error(),
	}
	archiveSource := "self_update_pull_failed"
	if isCancelled(err, parent, ctx) {
		entry.Outcome = history.OutcomeCancelled
		entry.Error = "self-update cancelled by operator during pull"
		archiveSource = "self_update_cancelled"
		op.state.MarkError("SELF_UPDATE_CANCELLED", entry.Error)
		fmt.Fprintln(op.logWriter, entry.Error)
	} else {
		op.state.MarkError("SELF_UPDATE_PULL_FAILED", fmt.Sprintf("self-update pull failed: %v", err))
		fmt.Fprintf(op.logWriter, "pull failed: %v\n", err)
	}
	d.appendHistory(entry)
	ArchiveUpdateLogToFailures(d.DataDir, op.updateID, archiveSource, op.logWriter)
	return err
}

func (d *Deps) recordPendingSelfUpdate(op *selfUpdateOperation) {
	d.appendHistory(history.Entry{
		ID:          op.updateID,
		StartedAt:   op.started,
		FinishedAt:  time.Now(),
		DurationMS:  time.Since(op.started).Milliseconds(),
		FromVersion: op.fromVersion,
		ToVersion:   op.toVersion,
		Outcome:     history.OutcomePending,
	})
}

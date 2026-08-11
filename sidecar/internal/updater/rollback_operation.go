package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/backup"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

// RunRollback performs the operator-triggered rollback to the newest backup.
func (d *Deps) RunRollback(ctx context.Context) error {
	started := time.Now()
	updateID := fmt.Sprintf("rbk-%d", started.UnixNano())
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

	fmt.Fprintf(logW, "=== Rollback %s started %s ===\n", updateID, started.Format(time.RFC3339))
	d.State.SetOpID(opID)
	d.State.SetUpdateID(updateID)
	fromVersion := d.CurrentVersion(ctx)
	backupPath, err := d.Backup.LatestBackup()
	if err != nil {
		err = fmt.Errorf("find latest backup: %w", err)
	} else if backupPath == "" {
		err = fmt.Errorf("find latest backup: no pre-update backup available")
	}
	rollbackVersion := ""
	if err == nil {
		rollbackVersion, err = backup.VersionFromPath(backupPath)
		if err != nil {
			err = fmt.Errorf("determine rollback version: %w", err)
		} else if expectedVersion(rollbackVersion) == "" {
			err = fmt.Errorf("determine rollback version: backup target %q is not a concrete version", rollbackVersion)
		}
	}
	if err == nil {
		fmt.Fprintf(logW, "rollback: current=%q target=%q\n", fromVersion, rollbackVersion)
		err = d.doRollback(ctx, backupPath, rollbackVersion, logW)
	}
	finished := time.Now()

	entry := history.Entry{
		ID:          updateID,
		StartedAt:   started,
		FinishedAt:  finished,
		DurationMS:  finished.Sub(started).Milliseconds(),
		FromVersion: fromVersion,
		ToVersion:   rollbackVersion,
		Outcome:     history.OutcomeRollback,
	}
	if err != nil {
		entry.Outcome = history.OutcomeError
		entry.Error = err.Error()
		d.appendHistory(entry)
		d.State.MarkError("ROLLBACK_FAILED", err.Error())
		fmt.Fprintf(logW, "=== Rollback %s FAILED: %v ===\n", updateID, err)
		return err
	}
	d.appendHistory(entry)
	fmt.Fprintf(logW, "=== Rollback %s SUCCESS ===\n", updateID)
	_ = d.State.TransitionTo(state.Idle, "")
	return nil
}

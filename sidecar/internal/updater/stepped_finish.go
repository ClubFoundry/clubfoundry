package updater

import (
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

func (op *steppedUpdateOperation) finish() error {
	finished := time.Now()
	entry := history.Entry{
		ID:          op.updateID,
		StartedAt:   op.started,
		FinishedAt:  finished,
		DurationMS:  finished.Sub(op.started).Milliseconds(),
		FromVersion: op.initial,
		ToVersion:   op.path[len(op.path)-1],
		Steps:       op.steps,
		// Hops records the complete requested path, including targets that were
		// not attempted after an earlier failure.
		Hops: append([]string(nil), op.path...),
	}
	if op.runErr == nil {
		return op.finishSuccess(entry)
	}
	return op.finishFailure(entry)
}

func (op *steppedUpdateOperation) finishSuccess(entry history.Entry) error {
	entry.Outcome = history.OutcomeSuccess
	op.deps.appendHistory(entry)
	fmt.Fprintf(
		op.logWriter,
		"=== Stepped update %s SUCCESS (%dms, %d hops) ===\n",
		op.updateID, entry.DurationMS, len(op.steps),
	)
	_ = op.deps.State.TransitionTo(state.Idle, "")
	if op.deps.Telemetry != nil {
		op.deps.Telemetry.FireAfterSettle(op.ctx, entry)
	}
	return nil
}

func (op *steppedUpdateOperation) finishFailure(entry history.Entry) error {
	if op.rollbackErr != nil {
		entry.Outcome = history.OutcomeError
		entry.Error = fmt.Sprintf(
			"failed at step %d/%d: %v; rollback to %s also failed: %v",
			op.failedAt+1, len(op.path), op.runErr, op.lastSuccessful, op.rollbackErr,
		)
	} else if op.lastSuccessful == op.initial {
		entry.Outcome = history.OutcomeError
		entry.Error = fmt.Sprintf(
			"failed at step %d/%d: %v (no successful steps, rolled back to %s)",
			op.failedAt+1, len(op.path), op.runErr, op.initial,
		)
	} else {
		entry.Outcome = history.OutcomeRollback
		entry.Error = fmt.Sprintf(
			"failed at step %d/%d: %v (stopped at last successful: %s)",
			op.failedAt+1, len(op.path), op.runErr, op.lastSuccessful,
		)
	}

	d := op.deps
	d.appendHistory(entry)
	errorCode := classifyError(op.runErr, op.rollbackErr)
	d.State.MarkError(errorCode, entry.Error)
	fmt.Fprintf(op.logWriter, "=== Stepped update %s FAILED: %s ===\n", op.updateID, entry.Error)
	source := "stepped_failed"
	if op.rollbackErr != nil {
		writeFailureBundle(d.DataDir, entry, errorCode, "rollback", op.log.logFilePath(), op.logWriter)
		source = "stepped_rollback_failed"
	} else if entry.Outcome == history.OutcomeRollback {
		d.State.ClearError()
		_ = d.State.TransitionTo(state.Idle, "")
		source = "stepped_partial_rollback"
	}
	ArchiveUpdateLogToFailures(d.DataDir, op.updateID, source, op.logWriter)
	return fmt.Errorf("%s", entry.Error)
}

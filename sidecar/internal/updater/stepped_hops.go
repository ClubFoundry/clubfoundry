package updater

import (
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

func (op *steppedUpdateOperation) runHops() {
	for index, target := range op.path {
		if !op.runHop(index, target) {
			return
		}
	}
}

func (op *steppedUpdateOperation) runHop(index int, target string) bool {
	d := op.deps
	stepStarted := time.Now()
	d.State.UpdateStep(&state.StepInfo{
		Index:       index + 1,
		Total:       len(op.path),
		FromVersion: op.lastSuccessful,
		ToVersion:   target,
	})
	d.State.UpdateDetail(fmt.Sprintf(
		"step %d/%d: %s → %s",
		index+1, len(op.path), op.lastSuccessful, target,
	))
	fmt.Fprintf(
		op.logWriter,
		"\n--- step %d/%d: %s → %s ---\n",
		index+1, len(op.path), op.lastSuccessful, target,
	)

	hopOpts := dockerops.PullOpts{
		LogWriter: op.logWriter,
		ProgressFn: func(downloaded, total int64, bps float64) {
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
		},
	}

	backupPath, err := d.doUpdate(op.ctx, op.lastSuccessful, target, hopOpts, op.logWriter)
	stepFinished := time.Now()
	step := history.Step{
		From:     op.lastSuccessful,
		To:       target,
		Duration: stepFinished.Sub(stepStarted).Milliseconds(),
	}
	if err != nil {
		step.Outcome = history.OutcomeError
		step.Error = err.Error()
		op.steps = append(op.steps, step)
		op.runErr = err
		op.failedAt = index
		fmt.Fprintf(op.logWriter, "step %d FAILED: %v — rolling back this step\n", index+1, err)
		op.rollbackErr = d.doRollback(op.ctx, backupPath, op.lastSuccessful, op.logWriter)
		if op.rollbackErr != nil {
			fmt.Fprintf(op.logWriter, "step %d rollback FAILED: %v\n", index+1, op.rollbackErr)
		}
		return false
	}

	step.Outcome = history.OutcomeSuccess
	op.steps = append(op.steps, step)
	op.lastSuccessful = target
	fmt.Fprintf(op.logWriter, "step %d success (%dms)\n", index+1, step.Duration)
	_ = d.Backup.PruneOld()
	return true
}

package updater

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

type steppedUpdateOperation struct {
	deps           *Deps
	ctx            context.Context
	path           []string
	started        time.Time
	updateID       string
	opID           string
	initial        string
	lastSuccessful string
	steps          []history.Step
	runErr         error
	rollbackErr    error
	failedAt       int
	log            *updateLog
	logWriter      io.Writer
}

func (d *Deps) beginSteppedUpdate(ctx context.Context, path []string) *steppedUpdateOperation {
	started := time.Now()
	op := &steppedUpdateOperation{
		deps:     d,
		ctx:      ctx,
		path:     path,
		started:  started,
		updateID: fmt.Sprintf("upd-stepped-%d", started.UnixNano()),
		opID:     newOpID(),
	}
	op.log = d.openUpdateLog(op.updateID, op.opID, state.KindMain)
	op.logWriter = op.log.writer()
	if d.State != nil {
		op.log.writeStateSnapshot("state-pre", d.State.Snapshot())
		d.State.RegisterChangeHook(op.log.hookFn())
	}
	fmt.Fprintf(op.logWriter, "=== Stepped update %s started %s ===\n", op.updateID, started.Format(time.RFC3339))
	fmt.Fprintf(op.logWriter, "op_id=%q path=%v\n", op.opID, path)
	return op
}

func (op *steppedUpdateOperation) close() {
	if op.deps.State != nil {
		op.deps.State.RegisterChangeHook(nil)
		op.log.writeStateSnapshot("state-post", op.deps.State.Snapshot())
	}
	op.log.close()
}

func (op *steppedUpdateOperation) start() error {
	d := op.deps
	if err := d.State.TransitionTo(
		state.Updating,
		fmt.Sprintf("step 1/%d: → %s", len(op.path), op.path[0]),
	); err != nil {
		return err
	}
	d.State.SetOpID(op.opID)
	d.State.SetUpdateID(op.updateID)
	d.State.SetTarget(op.path[len(op.path)-1])
	op.initial = d.CurrentVersion(op.ctx)
	op.lastSuccessful = op.initial
	return nil
}

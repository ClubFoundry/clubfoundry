package updater

import (
	"context"
	"fmt"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// RunSelfUpdate loads a sidecar image and schedules its recreation through a
// detached trampoline. The replacement process finalizes state from the
// trampoline sentinel because the current process cannot observe its own exit.
func (d *Deps) RunSelfUpdate(parent context.Context, targetVersion string, opts dockerops.PullOpts) error {
	op := d.beginSelfUpdate(targetVersion, opts.URL)
	defer op.close()

	if err := op.state.TransitionTo(state.Updating, "self-update: pulling new sidecar image"); err != nil {
		return err
	}
	op.state.SetOpID(op.opID)
	op.state.SetTarget(op.toVersion)
	op.state.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Downloading sidecar %s", op.toVersion))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	d.armCancel(cancel)
	defer d.disarmCancel()

	if err := d.pullSelfImage(ctx, parent, targetVersion, opts, op); err != nil {
		return err
	}
	d.recordPendingSelfUpdate(op)
	if err := d.validateSelfCompose(ctx, op); err != nil {
		return err
	}
	return d.spawnSelfTrampoline(ctx, op)
}

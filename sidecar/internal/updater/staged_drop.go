package updater

import (
	"fmt"

	"github.com/clubfoundry/updater/internal/state"
)

// DropStaged abandons the target but leaves the cached image for normal retention.
func (d *Deps) DropStaged() error {
	snap := d.State.Snapshot()
	if snap.Phase != state.Staged {
		return fmt.Errorf("nothing staged (phase=%s)", snap.Phase)
	}
	if err := d.State.TransitionTo(state.Idle, "Staged image dropped"); err != nil {
		return fmt.Errorf("drop staged: %w", err)
	}
	return nil
}

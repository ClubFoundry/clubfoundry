package updater

import (
	"context"
	"fmt"
)

// RunSteppedUpdate executes each target as a complete update cycle. A failed
// hop rolls back to the last version that completed successfully.
func (d *Deps) RunSteppedUpdate(ctx context.Context, path []string) error {
	if len(path) == 0 {
		return fmt.Errorf("empty update path")
	}
	op := d.beginSteppedUpdate(ctx, path)
	defer op.close()
	if err := op.start(); err != nil {
		return err
	}
	op.runHops()
	return op.finish()
}

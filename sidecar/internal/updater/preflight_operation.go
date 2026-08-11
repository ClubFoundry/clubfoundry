package updater

import (
	"context"
	"fmt"
	"io"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// preflight applies the public readiness verdict to an in-flight operation.
func (d *Deps) preflight(ctx context.Context, opts dockerops.PullOpts, logW io.Writer) error {
	d.State.UpdateSubStep(state.SubStepPreflight, "Checking disk space and download URL")
	fmt.Fprintf(logW, "[0/5] preflight: data dir=%q\n", preflightDataDir)

	r := d.Preflight(ctx, opts)
	for k, v := range r.Details {
		fmt.Fprintf(logW, "      %s: %s\n", k, v)
	}
	if !r.AllOK {
		switch {
		case !r.DiskOK:
			return fmt.Errorf("%s", r.Details["disk"])
		case !r.NetworkOK:
			return fmt.Errorf("%s", r.Details["network"])
		default:
			return fmt.Errorf("preflight failed: %+v", r.Details)
		}
	}
	return nil
}

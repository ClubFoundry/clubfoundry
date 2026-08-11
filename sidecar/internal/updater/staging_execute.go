package updater

import (
	"context"
	"fmt"
	"io"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// doStage performs preflight and image loading without changing the running service.
func (d *Deps) doStage(ctx context.Context, fromVersion, toVersion string, opts dockerops.PullOpts, logW io.Writer) error {
	if err := d.preflight(ctx, opts, logW); err != nil {
		return err
	}

	if opts.URL != "" {
		d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Downloading %s", toVersion))
		fmt.Fprintf(logW, "[stage] downloading tarball: %s\n", opts.URL)
	} else {
		d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Pulling %s from registry", toVersion))
		fmt.Fprintf(logW, "[stage] docker compose pull %s\n", d.Docker.MainServiceName())
	}
	if err := d.Docker.Pull(ctx, d.Docker.MainServiceName(), toVersion, opts); err != nil {
		return fmt.Errorf("pull image: %w", err)
	}

	d.State.UpdateSubStep(state.SubStepVerifying, "Image cached locally")
	fmt.Fprintf(logW, "[stage] image verified + loaded\n")
	return nil
}

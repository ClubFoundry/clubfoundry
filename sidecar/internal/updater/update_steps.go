package updater

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/state"
)

// doUpdate executes the forward path and returns the backup needed by failure
// recovery. Download and schema validation run before the service is stopped so
// the operator UI stays available during the slow, non-destructive work.
func (d *Deps) doUpdate(ctx context.Context, fromVersion, toVersion string, opts dockerops.PullOpts, logW io.Writer) (string, error) {
	// Preflight must complete before any destructive operation.
	if err := d.preflight(ctx, opts, logW); err != nil {
		return "", err
	}

	// Pull while the current service is still available.
	if opts.URL != "" {
		d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Downloading %s", toVersion))
		fmt.Fprintf(logW, "[1/5] downloading tarball: %s (main service still running)\n", opts.URL)
	} else {
		d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Pulling %s from registry", toVersion))
		fmt.Fprintf(logW, "[1/5] docker compose pull %s (main service still running)\n", d.Docker.MainServiceName())
	}
	if err := d.Docker.Pull(ctx, d.Docker.MainServiceName(), toVersion, opts); err != nil {
		// No destructive step has run, so there is no backup to return.
		return "", fmt.Errorf("pull image: %w", err)
	}

	// Pull performs the real integrity check; this state keeps the UI explicit.
	d.State.UpdateSubStep(state.SubStepVerifying, "Verifying image integrity")
	fmt.Fprintf(logW, "      image verified + loaded\n")

	// Validate candidate migrations against a database copy before Stop.
	if err := d.runSchemaDryrun(ctx, toVersion, logW); err != nil {
		return "", fmt.Errorf("schema dry-run: %w", err)
	}

	// Stop before backup so the SQLite files are consistent.
	d.State.UpdateSubStep(state.SubStepStopping, "Stopping current version")
	fmt.Fprintf(logW, "[2/5] stopping main service\n")
	if err := d.Docker.Stop(ctx, d.Docker.MainServiceName()); err != nil {
		return "", fmt.Errorf("stop main: %w", err)
	}

	d.State.UpdateSubStep(state.SubStepBackup, "Backing up database")
	fmt.Fprintf(logW, "[3/5] backup db for from-version %q\n", fromVersion)
	backupPath, err := d.Backup.CreateBackup(fromVersion)
	if err != nil {
		// Restore availability if backup creation fails after Stop.
		_ = d.Docker.Up(ctx, d.Docker.MainServiceName())
		return "", fmt.Errorf("backup db: %w", err)
	}
	fmt.Fprintf(logW, "      backup -> %s\n", backupPath)

	// Revalidate Compose immediately before recreating the service.
	if err := d.Docker.ValidateComposeForRecreate(ctx, d.Docker.MainServiceName()); err != nil {
		fmt.Fprintf(logW, "[4/5] compose pre-flight FAILED: %v\n", err)
		return backupPath, fmt.Errorf("compose pre-flight (main): %w", err)
	}
	d.State.UpdateSubStep(state.SubStepStarting, "Starting new version")
	fmt.Fprintf(logW, "[4/5] docker compose up --force-recreate %s\n", d.Docker.MainServiceName())
	if err := d.Docker.Up(ctx, d.Docker.MainServiceName()); err != nil {
		return backupPath, fmt.Errorf("docker compose up: %w", err)
	}

	// Health alone is insufficient: the reported version must match the target.
	d.State.UpdateSubStep(state.SubStepHealthCheck, "Waiting for application to start")
	fmt.Fprintf(logW, "[5/5] waiting for /health (up to %s)\n", d.effectiveStartup())
	waitCtx, cancel := context.WithTimeout(ctx, d.effectiveStartup())
	defer cancel()
	// Surface backend migration progress instead of a generic health spinner.
	onHealthProgress := func(r health.Report) {
		switch r.Phase {
		case "migrating":
			detail := "Running database migrations"
			if r.Migration != "" {
				detail = fmt.Sprintf("Running migration: %s", r.Migration)
			}
			d.State.UpdateSubStep(state.SubStepMigrating, detail)
			fmt.Fprintf(logW, "      backend phase=migrating file=%q\n", r.Migration)
		case "starting":
			d.State.UpdateSubStep(state.SubStepHealthCheck, "Application is starting up")
		}
	}
	healthInfo, err := d.Health.WaitHealthy(waitCtx, 2*time.Second, onHealthProgress)
	if err != nil {
		return backupPath, fmt.Errorf("post-update health: %w", err)
	}
	fmt.Fprintf(logW, "      /health ok — running version %q\n", healthInfo.Version)

	// Older backends may omit version; keep them compatible but log the gap.
	if healthInfo.Version == "" {
		fmt.Fprintf(logW, "      WARN: /health returned no version field — cannot verify version match\n")
	} else if expected := expectedVersion(toVersion); expected != "" && healthInfo.Version != expected {
		return backupPath, fmt.Errorf(
			"version mismatch after update: /health reports %q, expected %q (image ref may not have been rewritten, or an older layer was pulled from cache)",
			healthInfo.Version, expected,
		)
	}

	// A delayed second probe catches startup crashes and version drift.
	if err := d.smokeTest(ctx, healthInfo.Version, logW); err != nil {
		return backupPath, fmt.Errorf("post-update smoke test: %w", err)
	}
	return backupPath, nil
}

package updater

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/state"
)

// doApply runs the destructive portion of an update against an image already
// loaded by Stage.
func (d *Deps) doApply(ctx context.Context, fromVersion, toVersion string, logW io.Writer) (string, error) {
	d.State.UpdateSubStep(state.SubStepStopping, "Stopping current version")
	fmt.Fprintf(logW, "[apply 1/4] stopping main service\n")
	if err := d.Docker.Stop(ctx, d.Docker.MainServiceName()); err != nil {
		return "", fmt.Errorf("stop main: %w", err)
	}

	d.State.UpdateSubStep(state.SubStepBackup, "Backing up database")
	fmt.Fprintf(logW, "[apply 2/4] backup db for from-version %q\n", fromVersion)
	backupPath, err := d.Backup.CreateBackup(fromVersion)
	if err != nil {
		_ = d.Docker.Up(ctx, d.Docker.MainServiceName())
		return "", fmt.Errorf("backup db: %w", err)
	}
	fmt.Fprintf(logW, "       backup -> %s\n", backupPath)

	// Validate Compose before the recreate call while rollback still has a
	// consistent database backup available.
	if err := d.Docker.ValidateComposeForRecreate(ctx, d.Docker.MainServiceName()); err != nil {
		fmt.Fprintf(logW, "[apply 3/4] compose pre-flight FAILED: %v\n", err)
		return backupPath, fmt.Errorf("compose pre-flight (apply): %w", err)
	}
	d.State.UpdateSubStep(state.SubStepStarting, "Starting new version (staged image)")
	fmt.Fprintf(logW, "[apply 3/4] docker compose up --force-recreate %s (from staged image)\n", d.Docker.MainServiceName())
	if err := d.Docker.Up(ctx, d.Docker.MainServiceName()); err != nil {
		return backupPath, fmt.Errorf("docker compose up: %w", err)
	}

	d.State.UpdateSubStep(state.SubStepHealthCheck, "Waiting for application to start")
	fmt.Fprintf(logW, "[apply 4/4] waiting for /health (up to %s)\n", d.effectiveStartup())
	waitCtx, cancel := context.WithTimeout(ctx, d.effectiveStartup())
	defer cancel()
	healthInfo, err := d.Health.WaitHealthy(waitCtx, 2*time.Second, healthProgressFn(d, logW))
	if err != nil {
		return backupPath, fmt.Errorf("post-update health: %w", err)
	}
	fmt.Fprintf(logW, "       /health ok — running version %q\n", healthInfo.Version)

	if healthInfo.Version == "" {
		fmt.Fprintf(logW, "       WARN: /health returned no version field — cannot verify version match\n")
	} else if expected := expectedVersion(toVersion); expected != "" && healthInfo.Version != expected {
		return backupPath, fmt.Errorf(
			"version mismatch after apply: /health reports %q, expected %q",
			healthInfo.Version, expected,
		)
	}
	if err := d.smokeTest(ctx, healthInfo.Version, logW); err != nil {
		return backupPath, fmt.Errorf("post-apply smoke test: %w", err)
	}
	return backupPath, nil
}

func healthProgressFn(d *Deps, logW io.Writer) func(r health.Report) {
	return func(r health.Report) {
		switch r.Phase {
		case "migrating":
			detail := "Running database migrations"
			if r.Migration != "" {
				detail = fmt.Sprintf("Running migration: %s", r.Migration)
			}
			d.State.UpdateSubStep(state.SubStepMigrating, detail)
			fmt.Fprintf(logW, "       backend phase=migrating file=%q\n", r.Migration)
		case "starting":
			d.State.UpdateSubStep(state.SubStepHealthCheck, "Application is starting up")
		}
	}
}

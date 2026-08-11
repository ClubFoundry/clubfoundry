package updater

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// doRollback restores the backup and brings the old container back up.
// Called by RunUpdate on failure, and by the HTTP /rollback handler when
// an operator wants to revert manually.
func (d *Deps) doRollback(ctx context.Context, backupPath, fromVersion string, logW io.Writer) error {
	if err := d.State.TransitionTo(state.RollingBack, "restoring previous version"); err != nil {
		// Recovery remains best-effort even from an unexpected source phase.
	}
	d.State.UpdateSubStep(state.SubStepBackup, "Restoring database backup")
	if backupPath == "" {
		latest, err := d.Backup.LatestBackup()
		if err != nil {
			return fmt.Errorf("find latest backup: %w", err)
		}
		backupPath = latest
	}
	fmt.Fprintf(logW, "rollback: using backup %s\n", backupPath)

	_ = d.Docker.Stop(ctx, d.Docker.MainServiceName())

	if backupPath != "" {
		if err := d.Backup.RestoreBackup(backupPath); err != nil {
			return fmt.Errorf("restore backup: %w", err)
		}
	}

	if fromVersion != "" && fromVersion != "unknown" {
		// Prefer the exact prior version, then the retained fallback, then the
		// network. `:previous` may intentionally be one successful update older.
		repo := d.Docker.MainServiceName()
		previousRef := fmt.Sprintf("%s:previous", repo)
		versionedRef := fmt.Sprintf("%s:%s", repo, fromVersion)
		usedLocal := false
		if d.Docker.HasImage(ctx, versionedRef) {
			d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Restoring previous version %s from local versioned tag", fromVersion))
			fmt.Fprintf(logW, "rollback: using local %s (versioned tag from prior pull, exact match for fromVersion)\n", versionedRef)
			if err := d.Docker.SetServiceImage(repo, versionedRef); err != nil {
				return fmt.Errorf("rollback: set compose image to %s: %w", versionedRef, err)
			}
			usedLocal = true
		} else if d.Docker.HasImage(ctx, previousRef) {
			d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Restoring from local :previous tag (versioned tag for %s missing)", fromVersion))
			fmt.Fprintf(logW, "rollback: %s missing — falling back to %s (Phase B retained tag, may be one version older than fromVersion)\n", versionedRef, previousRef)
			if err := d.Docker.SetServiceImage(repo, previousRef); err != nil {
				return fmt.Errorf("rollback: set compose image to %s: %w", previousRef, err)
			}
			usedLocal = true
		}
		if !usedLocal {
			d.State.UpdateSubStep(state.SubStepDownloading, fmt.Sprintf("Re-pulling previous version %s", fromVersion))
			fmt.Fprintf(logW, "rollback: no local rollback target (%s + :previous both absent) — re-pulling %s from registry\n", versionedRef, fromVersion)
			if err := d.Docker.Pull(ctx, d.Docker.MainServiceName(), fromVersion, dockerops.PullOpts{LogWriter: logW}); err != nil {
				return fmt.Errorf("revert compose + re-pull %s during rollback: %w", fromVersion, err)
			}
		}
	}

	d.State.UpdateSubStep(state.SubStepStarting, "Starting previous version")
	if err := d.Docker.Up(ctx, d.Docker.MainServiceName()); err != nil {
		return fmt.Errorf("restart after restore: %w", err)
	}

	d.State.UpdateSubStep(state.SubStepHealthCheck, "Waiting for previous version to start")
	waitCtx, cancel := context.WithTimeout(ctx, d.effectiveStartup())
	defer cancel()
	healthInfo, err := d.Health.WaitHealthy(waitCtx, 2*time.Second)
	if err != nil {
		return fmt.Errorf("post-rollback health: %w", err)
	}
	// A healthy rollback still fails if Docker restored the wrong version.
	if expected := expectedVersion(fromVersion); expected != "" {
		if healthInfo.Version == "" {
			fmt.Fprintf(logW, "rollback: WARN /health returned no version field — cannot verify version match\n")
		} else if healthInfo.Version != expected {
			return fmt.Errorf(
				"post-rollback version mismatch: /health reports %q, expected %q (the image at compose's %s tag is not what %s pointed at before the failed update — rollback target was probably overwritten)",
				healthInfo.Version, expected, fromVersion, fromVersion,
			)
		} else {
			fmt.Fprintf(logW, "rollback: /health verified version %q\n", healthInfo.Version)
		}
	}
	return nil
}

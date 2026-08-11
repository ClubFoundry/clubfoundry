package updater

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

// validateSelfCompose keeps the current sidecar alive when the trampoline
// would be unable to resolve or recreate its Compose service.
func (d *Deps) validateSelfCompose(ctx context.Context, op *selfUpdateOperation) error {
	service := d.Docker.UpdaterServiceName()
	op.state.UpdateSubStep(state.SubStepPreflight, "Validating compose configuration")
	fmt.Fprintf(op.logWriter, "[pre-flight] validating compose for service=%s\n", service)
	if err := d.Docker.ValidateComposeForRecreate(ctx, service); err != nil {
		message := fmt.Sprintf("compose pre-flight failed (sidecar still running): %v", err)
		fmt.Fprintf(op.logWriter, "[pre-flight] FAILED: %s\n", message)
		op.log.appendPhaseExtras(state.Updating, state.SubStepPreflight,
			"compose_preflight_failed", map[string]any{"error": err.Error(), "service": service})
		op.state.MarkError("COMPOSE_VALIDATION_FAILED", message)
		d.appendHistory(history.Entry{
			ID:          op.updateID,
			StartedAt:   op.started,
			FinishedAt:  time.Now(),
			FromVersion: op.fromVersion,
			ToVersion:   op.toVersion,
			Outcome:     history.OutcomeError,
			Error:       message,
		})
		ArchiveUpdateLogToFailures(d.DataDir, op.updateID, "compose_preflight_failed", op.logWriter)
		return fmt.Errorf("%s", message)
	}
	fmt.Fprintln(op.logWriter, "[pre-flight] OK")
	return nil
}

func (d *Deps) spawnSelfTrampoline(ctx context.Context, op *selfUpdateOperation) error {
	op.state.UpdateSubStep(state.SubStepSpawning, "Recreating sidecar container")
	trampolineID := newTrampolineID()
	sentinelPath := state.SentinelPath(d.DataDir, trampolineID)
	stdoutPath, stderrPath := trampolineLogPaths(op.log.path())
	opts := dockerops.TrampolineOpts{
		SentinelPath:  sentinelPath,
		TrampolineID:  trampolineID,
		TargetVersion: op.toVersion,
		OpID:          op.opID,
		LogStdoutPath: stdoutPath,
		LogStderrPath: stderrPath,
	}

	log.Printf("self-update: spawning trampoline id=%s sentinel=%s target=%s",
		trampolineID, sentinelPath, op.toVersion)
	fmt.Fprintf(op.logWriter, "spawning trampoline id=%s sentinel=%s\n", trampolineID, sentinelPath)
	if stdoutPath != "" {
		fmt.Fprintf(op.logWriter, "trampoline output → %s + %s\n", stdoutPath, stderrPath)
	}
	op.log.appendPhaseExtras(state.Updating, state.SubStepSpawning,
		"trampoline_spawn",
		map[string]any{
			"trampoline_id":   trampolineID,
			"sentinel_path":   sentinelPath,
			"target":          op.toVersion,
			"log_stdout_path": stdoutPath,
			"log_stderr_path": stderrPath,
		})

	service := d.Docker.UpdaterServiceName()
	if err := d.Docker.SpawnRecreateTrampoline(ctx, service, 5, opts); err != nil {
		fmt.Fprintf(op.logWriter, "trampoline spawn failed: %v\n", err)
		d.appendHistory(history.Entry{
			ID:          op.updateID + "-up-fail",
			StartedAt:   time.Now(),
			FinishedAt:  time.Now(),
			FromVersion: op.fromVersion,
			ToVersion:   op.toVersion,
			Outcome:     history.OutcomeError,
			Error:       err.Error(),
		})
		op.state.MarkError("SELF_UPDATE_RECREATE_FAILED", fmt.Sprintf("self-update recreate failed: %v", err))
		ArchiveUpdateLogToFailures(d.DataDir, op.updateID, "self_update_recreate_failed", op.logWriter)
		return err
	}
	fmt.Fprintln(op.logWriter, "trampoline spawned successfully — sidecar will be killed by recreate")
	return nil
}

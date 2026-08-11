package main

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

func initializeOperationStates(dataDir, runningVersion string) (*state.State, *state.State) {
	mainState := state.NewKindAware(state.KindMain, dataDir)
	selfState := state.NewKindAware(state.KindSelf, dataDir)

	for _, current := range []*state.State{mainState, selfState} {
		snap := current.Snapshot()
		if snap.Phase != state.Idle {
			log.Printf("state restored: kind=%s phase=%s sub_step=%s op_id=%s target=%s detail=%q",
				current.Kind(), snap.Phase, snap.SubStep, snap.OpID, snap.TargetVersion, snap.Detail)
		}
		if err := current.PersistErr(); err != nil {
			log.Printf("state persistence error (kind=%s): %v", current.Kind(), err)
		}
	}

	// Sweep diagnostics at boot for installations that update infrequently.
	updater.SweepDiagnosticRetention(dataDir)

	// Finalize recent self-update sentinels left by the trampoline.
	if n, err := state.FinalizeSelfFromSentinels(dataDir, runningVersion, selfState); err != nil {
		log.Printf("trampoline sentinel finalize: %v", err)
	} else if n > 0 {
		log.Printf("trampoline sentinels: finalized %d entr(y|ies)", n)
	}
	// Preserve failed trampoline artifacts for diagnostics.
	maybeArchiveErrorState(dataDir, selfState, "trampoline_sentinel_error")

	// Poll for a delayed sentinel so a restored self-update is not mistaken
	// for a stuck operation.
	if snap := selfState.Snapshot(); snap.Phase != state.Idle && snap.Phase != state.Error {
		raceCtx, raceCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		go func() {
			defer raceCancel()
			state.WaitForSelfFinalize(raceCtx, dataDir, runningVersion, selfState, 2*time.Second)
			// Archive failures reported by sentinels that arrived after boot.
			maybeArchiveErrorState(dataDir, selfState, "trampoline_poll_error")
		}()
	}

	return mainState, selfState
}

package state

import (
	"context"
	"log"
	"time"
)

// WaitForSelfFinalize polls until the self state settles or ctx ends.
func WaitForSelfFinalize(ctx context.Context, dataDir, currentSidecarVersion string, selfState *State, pollInterval time.Duration) {
	if selfState == nil || dataDir == "" {
		return
	}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap := selfState.Snapshot()
			if snap.Phase == Idle || snap.Phase == Error {
				return
			}
			n, err := FinalizeSelfFromSentinels(dataDir, currentSidecarVersion, selfState)
			if err != nil {
				log.Printf("sentinel: poll finalize error: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("sentinel: poll-finalized %d entr(y|ies) (race window closed)", n)
				return
			}
		}
	}
}

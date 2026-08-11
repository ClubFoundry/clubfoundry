package main

import (
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

// maybeArchiveErrorState preserves artifacts for a restored failed operation.
func maybeArchiveErrorState(dataDir string, s *state.State, source string) {
	if s == nil {
		return
	}
	snap := s.Snapshot()
	if snap.Phase != state.Error || snap.UpdateID == "" {
		return
	}
	updater.ArchiveUpdateLogToFailures(dataDir, snap.UpdateID, source, nil)
}

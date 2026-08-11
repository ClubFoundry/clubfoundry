package main

import (
	"net/http"
	"path/filepath"

	"github.com/clubfoundry/updater/internal/handlers"
	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
)

func registerRuntimeRoutes(dataDir, runningVersion string, runtime runtimeDependencies, mainState, selfState *state.State) (*http.ServeMux, *recoveryhistory.EventStore) {
	// Persist recovery actions so the main UI can report them to operators.
	recoveryEvents := recoveryhistory.NewStore(dataDir)
	mux := http.NewServeMux()
	handlers.Register(mux, handlers.Deps{
		State:          mainState,
		SelfState:      selfState,
		Version:        runningVersion,
		Updater:        runtime.updater,
		History:        runtime.history,
		ConfigStore:    runtime.config,
		Cloud:          runtime.cloud,
		LogDir:         filepath.Join(dataDir, "update-logs"),
		DataDir:        dataDir,
		Docker:         runtime.docker,
		RecoveryEvents: recoveryEvents,
	})
	return mux, recoveryEvents
}

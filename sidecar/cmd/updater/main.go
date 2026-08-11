// Command updater runs the ClubFoundry auto-update sidecar.
package main

import (
	"context"
	"log"

	"github.com/clubfoundry/updater/internal/auth"
	"github.com/clubfoundry/updater/internal/state"
)

// version is set at build time and defaults to "dev" for local runs.
var version = "dev"

func main() {
	settings := loadRuntimeSettings()

	// Filesystem errors are fatal because silently disabling auth is unsafe.
	authToken, err := auth.Init(settings.DataDir)
	if err != nil {
		log.Fatalf("auth.Init failed: %v", err)
	}

	mainState, selfState := initializeOperationStates(settings.DataDir, version)
	runtime := initializeRuntimeDependencies(settings.DataDir, version, mainState, selfState)

	runStartupBootstrap(mainState, runtime.docker, runtime.cloud, version)
	inspectComposeProject(runtime.docker)

	// Resume a queued main update before exposing the HTTP API. The start signal
	// prevents another update from racing the Idle-to-Updating transition.
	resumeStarted := make(chan struct{})
	if snap := mainState.Snapshot(); snap.Phase == state.Idle && snap.PendingMainTarget != "" {
		log.Printf("chained main-update resume: queued target=%s — firing RunUpdate", snap.PendingMainTarget)
		go runChainedMainResume(runtime.updater, mainState, runtime.cloud, runtime.config, snap.PendingMainTarget, resumeStarted)
	} else {
		close(resumeStarted)
	}

	mux, recoveryEvents := registerRuntimeRoutes(settings.DataDir, version, runtime, mainState, selfState)

	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	defer runtimeCancel()
	startBackgroundServices(runtimeCtx, settings.DataDir, runtime, mainState, selfState, recoveryEvents)

	// Log every request before authentication; /health remains anonymous.
	srv := newHTTPServer(settings.Addr, logMiddleware(authToken.Middleware(mux)))
	awaitResumeStart(resumeStarted)
	serveUntilSignal(srv, version)
}

package main

import (
	"log"
	"path/filepath"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/config"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

type runtimeDependencies struct {
	docker  dockerops.Config
	history *history.Log
	config  *config.Store
	health  *health.Checker
	cloud   *cloud.Client
	updater *updater.Deps
}

func initializeRuntimeDependencies(dataDir, selfVersion string, mainState, selfState *state.State) runtimeDependencies {
	dockerCfg := dockerops.DefaultConfig()
	backupCfg := backupConfigForDataDir(dataDir)
	histLog := history.New(filepath.Join(dataDir, "updater-history.json"))
	// A running replacement proves that pending trampoline updates succeeded.
	if n, err := histLog.FinalizePendingSelfUpdates(); err != nil {
		log.Printf("history finalize-pending: %v", err)
	} else if n > 0 {
		log.Printf("history: finalized %d pending self-update entr(y|ies) to success", n)
	}
	confStore := config.NewStore(filepath.Join(dataDir, "updater-config.json"))
	healthCheck := health.DefaultChecker()
	// Log the resolved health URL because the installer may select another port.
	log.Printf("monitor: resolved main /health probe URL = %s", healthCheck.URL)

	// The stateless cloud client is shared by all background services.
	cloudClient := cloud.DefaultClient()
	tel := telemetryForCloud(cloudClient, healthCheck, dataDir)
	upd := &updater.Deps{
		Docker:      dockerCfg,
		Backup:      backupCfg,
		Health:      healthCheck,
		History:     histLog,
		State:       mainState,
		SelfState:   selfState,
		DataDir:     dataDir,
		LogDir:      filepath.Join(dataDir, "update-logs"),
		Telemetry:   tel,
		SelfVersion: selfVersion,
		// The adapter avoids coupling the updater package to the cloud client.
		Cloud: cloudVersionAdapter{cli: cloudClient},
	}

	return runtimeDependencies{
		docker:  dockerCfg,
		history: histLog,
		config:  confStore,
		health:  healthCheck,
		cloud:   cloudClient,
		updater: upd,
	}
}

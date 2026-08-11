package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/clubfoundry/updater/internal/bootstrap"
	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

func runStartupBootstrap(mainState *state.State, dockerCfg dockerops.Config, cloudClient *cloud.Client, selfVersion string) {
	// Bootstrap only when no persisted main operation is in progress.
	if snap := mainState.Snapshot(); snap.Phase == state.Idle {
		bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		bootstrapDeps := bootstrap.Deps{
			Docker:      dockerCfg,
			Cloud:       cloudClient,
			State:       mainState,
			Channel:     os.Getenv("CLUBFOUNDRY_BOOTSTRAP_CHANNEL"),
			SelfVersion: selfVersion,
		}
		if err := bootstrapDeps.Run(bootstrapCtx); err != nil {
			log.Printf("bootstrap: %v (continuing — operator can retry via /status UI)", err)
			mainState.MarkError("BOOTSTRAP_FAILED", err.Error())
		}
		bootstrapCancel()
	} else {
		log.Printf("bootstrap: skipped — restored main-state phase=%s, leaving for operator/watchdog", mainState.Snapshot().Phase)
	}
}

func inspectComposeProject(dockerCfg dockerops.Config) {
	// Anchor Compose to one project name across working directories.
	composePath := filepath.Join(dockerCfg.ComposeDir, "docker-compose.yml")
	if changed, err := bootstrap.EnsureProjectName(composePath, "clubfoundry"); err != nil {
		log.Printf("compose project-name anchor: %v (continuing — drift detection below will surface it)", err)
	} else if changed {
		log.Printf("compose project-name anchor: patched %s with `name: clubfoundry` (.bak saved)", composePath)
	}

	// Report Compose project drift without changing running containers.
	driftCtx, driftCancel := context.WithTimeout(context.Background(), 15*time.Second)
	driftReport := bootstrap.ReportComposeProjectDrift(driftCtx, "", "clubfoundry", nil)
	driftCancel()
	bootstrap.LogDriftReport(driftReport)
}

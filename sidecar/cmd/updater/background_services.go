package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/clubfoundry/updater/internal/footprint"
	"github.com/clubfoundry/updater/internal/monitor"
	"github.com/clubfoundry/updater/internal/poller"
	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
)

func startBackgroundServices(ctx context.Context, dataDir string, runtime runtimeDependencies, mainState, selfState *state.State, recoveryEvents *recoveryhistory.EventStore) {
	// Poll for updates and recalled versions until shutdown.
	go (&poller.Deps{
		Cloud:   runtime.cloud,
		Config:  runtime.config,
		Updater: runtime.updater,
		State:   mainState,
	}).Run(ctx)

	// Monitor main-app health and apply the configured recovery policy.
	mon := monitor.New(runtime.health, runtime.docker, mainState, runtime.cloud)
	mon.SetRollbackTrigger(runtime.updater)
	// Allow same-version reinstall as the final recovery step before halt.
	mon.SetReinstallTrigger(runtime.updater)
	// Record monitor actions for the diagnostics UI.
	mon.SetEventStore(recoveryEvents)
	// Apply the same stale-error recovery policy to self-update state.
	mon.SetSelfState(selfState)
	mon.SetAppVersionGetter(func() string {
		// Version detection is best-effort during early startup.
		if runtime.updater == nil {
			return ""
		}
		versionCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return runtime.updater.CurrentVersion(versionCtx)
	})
	go mon.Run(ctx)

	// Watch each operation kind independently for expired sub-steps.
	go runWatchdog(ctx, dataDir, []*state.State{mainState, selfState}, runtime.updater)

	// Prune stale images using fresh settings on each cycle.
	if os.Getenv("CLUBFOUNDRY_DISABLE_AUTO_PRUNE") != "1" {
		footprint.StartDaemon(
			ctx,
			runtime.docker,
			func() footprint.PruneConfig {
				set, _, _ := runtime.config.Load()
				return footprint.PruneConfig{
					Enabled:      !set.AutoPruneOptOut,
					GraceDays:    set.AutoPruneGraceDays,
					KeepVersions: set.AutoPruneKeepVersions,
					Repos: []string{
						runtime.docker.MainServiceName(),
						runtime.docker.UpdaterServiceName(),
					},
					LogDir: filepath.Join(dataDir, "update-logs"),

					BuildCacheEnabled: !set.AutoPruneBuildCacheOptOut,
					BuildCacheKeepGB:  set.AutoPruneBuildCacheKeepGB,
					BuildCacheAgeDays: set.AutoPruneBuildCacheAgeDays,
				}
			},
			5*time.Minute,
			24*time.Hour,
			log.Printf,
		)
	}
}

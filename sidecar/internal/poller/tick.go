package poller

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

func (d *Deps) tick(ctx context.Context) {
	// Frozen-version test mode short-circuits before any cloud call.
	if frozen, frozenVer := sidecarIsFrozen(); frozen {
		frozenLogOnce.Do(func() {
			log.Printf("poller: CLM_SIDECAR_FROZEN_VERSION=%q set — auto-update loop disabled for this process boot", frozenVer)
		})
		return
	}
	// TrueNAS Apps owns docker compose pull/up in catalog-managed mode.
	if catalogManaged() {
		catalogManagedLogOnce.Do(func() {
			log.Printf("poller: CLM_UPDATE_MODE=truenas_apps — auto-update loop disabled (TrueNAS Apps owns image lifecycle)")
		})
		return
	}
	if d.State == nil || d.Updater == nil || d.Config == nil || d.Cloud == nil {
		return
	}
	if snap := d.State.Snapshot(); snap.Phase != state.Idle {
		return
	}

	curVer := d.Updater.CurrentVersion(ctx)
	set, _, _ := d.Config.Load()

	resp, err := d.Cloud.CheckUpdates(ctx, curVer, set.Channel)
	if err != nil {
		log.Printf("poller: cloud check failed: %v", err)
		return
	}
	if resp == nil {
		return // BaseURL not configured: air-gapped install
	}

	// Recall takes priority over the maintenance window.
	if cloud.IsRecalled(resp, curVer) && resp.RollbackTo != "" {
		log.Printf("poller: current version %s recalled, rolling back to %s", curVer, resp.RollbackTo)
		go func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			_ = d.Updater.RunUpdate(runCtx, resp.RollbackTo, dockerops.PullOpts{})
		}()
		return
	}

	if !set.AutoUpdate {
		return
	}
	if !insideWindow(time.Now().UTC(), set.UpdateWindow) {
		return
	}

	// Update the sidecar before the main app. Strict-newer comparison prevents
	// downgrades while release metadata is still converging.
	if d.Updater.SelfVersion != "" && d.Updater.SelfVersion != "dev" &&
		resp.UpdaterVersion != "" && updater.IsStrictlyNewerSidecarVersion(d.Updater.SelfVersion, resp.UpdaterVersion) &&
		resp.UpdaterDownloadUrl != "" && resp.UpdaterDownloadSha256 != "" {
		log.Printf("poller: sidecar self-update %s → %s via tarball %s (mirrors=%d)",
			d.Updater.SelfVersion, resp.UpdaterVersion, resp.UpdaterDownloadUrl, len(resp.UpdaterDownloadUrls))
		go func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			_ = d.Updater.RunSelfUpdate(runCtx, resp.UpdaterVersion, dockerops.PullOpts{
				URL:    resp.UpdaterDownloadUrl,
				URLs:   resp.UpdaterDownloadUrls,
				Sha256: resp.UpdaterDownloadSha256,
			})
		}()
		return // RunSelfUpdate recreates this container.
	}

	if resp.CurrentIsLatest {
		return
	}

	// Prefer the verified tarball direct jump when the server provides it.
	if resp.DownloadUrl != "" && resp.DownloadSha256 != "" {
		log.Printf("poller: auto-update %s → %s via tarball %s (mirrors=%d)", curVer, resp.Latest, resp.DownloadUrl, len(resp.DownloadUrls))
		go func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			_ = d.Updater.RunUpdate(runCtx, resp.Latest, dockerops.PullOpts{
				URL:    resp.DownloadUrl,
				URLs:   resp.DownloadUrls,
				Sha256: resp.DownloadSha256,
			})
		}()
		return
	}

	if resp.UpdatePath == nil {
		return
	}

	log.Printf("poller: auto-update %s → %s via %d step(s)", resp.UpdatePath.From, resp.UpdatePath.To, len(resp.UpdatePath.Path))
	go func(path []string) {
		runCtx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		defer cancel()
		_ = d.Updater.RunSteppedUpdate(runCtx, path)
	}(resp.UpdatePath.Path)
}

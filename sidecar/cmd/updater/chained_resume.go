package main

import (
	"context"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/config"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

// runChainedMainResume resolves and starts a main update queued before sidecar
// recreation. The queue remains intact when cloud metadata is unavailable.
func runChainedMainResume(upd *updater.Deps, mainState *state.State, cloudClient *cloud.Client, confStore *config.Store, target string, resumeStarted chan<- struct{}) {
	signaled := false
	signal := func() {
		if !signaled {
			signaled = true
			close(resumeStarted)
		}
	}
	defer signal()

	pullOpts := dockerops.PullOpts{}
	cloudOK := false
	if cloudClient != nil && cloudClient.BaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		channel := "stable"
		if confStore != nil {
			if set, _, err := confStore.Load(); err == nil && set.Channel != "" {
				channel = set.Channel
			}
		}
		current := ""
		if upd != nil {
			current = upd.CurrentVersion(ctx)
		}
		resp, err := cloudClient.CheckUpdates(ctx, current, channel)
		cancel()
		if err == nil && resp != nil && resp.DownloadUrl != "" && resp.DownloadSha256 != "" {
			pullOpts.URL = resp.DownloadUrl
			pullOpts.Sha256 = resp.DownloadSha256
			if len(resp.DownloadUrls) > 0 {
				pullOpts.URLs = resp.DownloadUrls
			}
			cloudOK = true
		} else if err != nil {
			log.Printf("chained main-update resume: cloud lookup failed (%v) — preserving queue, operator can retry", err)
		} else {
			log.Printf("chained main-update resume: cloud advertised no DownloadUrl/Sha256 — preserving queue, operator can retry")
		}
	}

	if !cloudOK {
		return
	}

	mainState.ClearPendingMainTarget()
	// Signal immediately before RunUpdate claims the Idle state so the HTTP
	// listener cannot normally admit a competing update first.
	signal()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := upd.RunUpdate(ctx, target, pullOpts); err != nil {
		log.Printf("chained main-update resume: RunUpdate failed: %v", err)
	}
}

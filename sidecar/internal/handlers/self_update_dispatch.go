package handlers

import (
	"context"
	"log"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// resolveSelfUpdateDispatch reads the sidecar artifact published by /api/update.
// Missing cloud data preserves the existing registry-pull fallback.
func resolveSelfUpdateDispatch(parent context.Context, d Deps) (dockerops.PullOpts, string) {
	if d.Cloud == nil || d.Cloud.BaseURL == "" {
		return dockerops.PullOpts{}, ""
	}
	channel := "stable"
	if d.ConfigStore != nil {
		if set, _, err := d.ConfigStore.Load(); err == nil && set.Channel != "" {
			channel = set.Channel
		}
	}
	current := ""
	if d.Updater != nil {
		current = d.Updater.CurrentVersion(parent)
	}
	ctx, cancel := context.WithTimeout(parent, updateDecisionTimeout)
	defer cancel()
	resp, err := d.Cloud.CheckUpdates(ctx, current, channel)
	if err != nil || resp == nil {
		if err != nil {
			log.Printf("self-update: cloud lookup failed (%v); fallback to registry pull", err)
		}
		return dockerops.PullOpts{}, ""
	}
	if resp.UpdaterDownloadUrl == "" || resp.UpdaterDownloadSha256 == "" {
		return dockerops.PullOpts{}, resp.UpdaterVersion
	}
	opts := dockerops.PullOpts{
		URL:    resp.UpdaterDownloadUrl,
		Sha256: resp.UpdaterDownloadSha256,
	}
	if len(resp.UpdaterDownloadUrls) > 0 {
		opts.URLs = resp.UpdaterDownloadUrls
	}
	return opts, resp.UpdaterVersion
}

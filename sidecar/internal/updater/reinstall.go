package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// ErrCloudUnavailable means recovery metadata cannot be fetched.
var ErrCloudUnavailable = fmt.Errorf("cloud client unavailable")

// ErrReinstallNotApplicable means rollback or manual recovery is safer.
var ErrReinstallNotApplicable = fmt.Errorf("reinstall not applicable")

// RunReinstallCurrent reloads the running version from a verified cloud
// artifact. It reuses the normal backup, health, rollback, and history flow.
func (d *Deps) RunReinstallCurrent(ctx context.Context) error {
	if d.Cloud == nil {
		return ErrCloudUnavailable
	}
	current := d.CurrentVersion(ctx)
	if current == "" || current == "unknown" {
		return fmt.Errorf("%w: cannot determine current version", ErrReinstallNotApplicable)
	}
	// Recovery lookup must not hang an already degraded installation.
	lookupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	meta, err := d.Cloud.FetchVersionMetadata(lookupCtx, current)
	if err != nil {
		return fmt.Errorf("fetch version metadata for %s: %w", current, err)
	}
	if meta == nil {
		return ErrCloudUnavailable
	}
	if meta.Recalled {
		// A recalled version must be rolled back, never reinstalled.
		return fmt.Errorf("%w: version %s is recalled (rollback recommended)", ErrReinstallNotApplicable, current)
	}
	if meta.DownloadUrl == "" || meta.DownloadSha256 == "" {
		return fmt.Errorf("%w: cloud returned no download URL/sha for %s", ErrReinstallNotApplicable, current)
	}

	// Prefer the mirror list while preserving single-URL metadata compatibility.
	urls := meta.DownloadUrls
	if len(urls) == 0 {
		urls = []string{meta.DownloadUrl}
	}
	opts := dockerops.PullOpts{
		URL:    urls[0],
		URLs:   urls,
		Sha256: meta.DownloadSha256,
	}

	return d.RunUpdate(ctx, current, opts)
}

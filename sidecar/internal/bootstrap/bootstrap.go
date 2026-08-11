// Package bootstrap creates a first Compose installation from a standalone
// sidecar. It resolves verified images, writes the initial configuration,
// starts the main service, and waits for health.
//
// Existing Compose files, managed containers, or healthy external installs
// make bootstrap a no-op. Failures are exposed through sidecar state without
// stopping its HTTP server.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// Run bootstraps a fresh installation. It returns nil after success or a skip.
func (d Deps) Run(ctx context.Context) error {
	composePath := filepath.Join(d.Docker.ComposeDir, "docker-compose.yml")

	// Skip 1: compose file already present — operator pre-configured.
	if _, err := os.Stat(composePath); err == nil {
		return nil
	}

	// Skip 2: main container already exists by name. Legacy docker-run
	// install (clm/clubfoundry) or prior partial bootstrap.
	exists, err := dockerops.ContainerExistsByName(ctx, d.Docker, d.Docker.MainService)
	if err != nil {
		return fmt.Errorf("probe main container: %w", err)
	}
	if exists {
		return nil
	}

	// Skip 3: main /health already answers — running outside Docker.
	if mainHealthy(ctx, d.healthURLOrDefault(), 2*time.Second) {
		return nil
	}

	// We're going to bootstrap. Surface progress on /status. State
	// transitions out of Idle into Checking — phase is reset to Idle
	// at the end of Run regardless of outcome.
	_ = d.State.TransitionTo(state.Checking, "Bootstrap: contacting cloud")
	defer func() { _ = d.State.TransitionTo(state.Idle, "") }()

	if d.Cloud == nil || d.Cloud.BaseURL == "" {
		return errors.New("bootstrap: CLUBFOUNDRY_CLOUD_URL not set")
	}

	channel := d.Channel
	if channel == "" {
		channel = "stable"
	}

	// "0.0.0" sentinel signals "no main installed yet" so the Worker
	// returns the latest stable rather than computing an update path
	// from a real version.
	resp, err := d.Cloud.CheckUpdates(ctx, "0.0.0", channel)
	if err != nil {
		return fmt.Errorf("cloud /api/update: %w", err)
	}
	if resp == nil || resp.Latest == "" {
		return errors.New("bootstrap: cloud returned empty version")
	}

	urls := resp.DownloadUrls
	if len(urls) == 0 && resp.DownloadUrl != "" {
		urls = []string{resp.DownloadUrl}
	}
	if len(urls) == 0 || resp.DownloadSha256 == "" {
		return fmt.Errorf("bootstrap: cloud returned %s but no download URL/sha256", resp.Latest)
	}

	d.State.UpdateDetail(fmt.Sprintf("Bootstrap: writing compose for %s", resp.Latest))
	if err := os.MkdirAll(d.Docker.ComposeDir, 0o755); err != nil {
		return fmt.Errorf("bootstrap: mkdir %s: %w", d.Docker.ComposeDir, err)
	}
	dataDir := os.Getenv("CLUBFOUNDRY_DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("bootstrap: mkdir %s: %w", dataDir, err)
	}
	if err := WriteComposeFile(composePath, ComposeParams{
		MainImage:       fmt.Sprintf("clubfoundry:%s", resp.Latest),
		MainService:     d.Docker.MainService,
		UpdaterImage:    fmt.Sprintf("clubfoundry-updater:%s", orDev(d.SelfVersion)),
		UpdaterService:  d.Docker.UpdaterService,
		HostDataDir:     hostDataDir(),
		HostComposeFile: hostComposePath(composePath),
		CloudURL:        d.Cloud.BaseURL,
	}); err != nil {
		return fmt.Errorf("bootstrap: write compose: %w", err)
	}

	envPath := filepath.Join(dataDir, ".env")
	if err := WriteEnvTemplateIfMissing(envPath); err != nil {
		return fmt.Errorf("bootstrap: write .env: %w", err)
	}
	// The main service runs as UID 100 GID 101 and writes secrets and the
	// database on first boot. Apply ownership after creating .env so every
	// required file is readable and the service can write to the bind mount.
	//
	// Ownership errors remain non-fatal for unprivileged sidecar deployments.
	if err := chownRecursive(dataDir, 100, 101); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: chown %s 100:101 failed (continuing): %v\n", dataDir, err)
	}

	// Pull = HEAD-probe mirrors → fastest first → sha-verified docker load
	// → SetServiceImage rewrite of the compose image: line. Existing
	// progress + error-classification path used by every regular update.
	d.State.UpdateDetail(fmt.Sprintf("Bootstrap: downloading %s", resp.Latest))
	if err := d.Docker.Pull(ctx, d.Docker.MainService, resp.Latest, dockerops.PullOpts{
		URLs:   urls,
		Sha256: resp.DownloadSha256,
	}); err != nil {
		return fmt.Errorf("bootstrap: pull main: %w", err)
	}

	d.State.UpdateDetail("Bootstrap: starting main container")
	if err := d.Docker.Up(ctx, d.Docker.MainService); err != nil {
		return fmt.Errorf("bootstrap: compose up main: %w", err)
	}

	d.State.UpdateDetail("Bootstrap: waiting for main /health")
	if err := waitMainHealthy(ctx, d.healthURLOrDefault(), 120*time.Second); err != nil {
		return fmt.Errorf("bootstrap: post-up health: %w", err)
	}

	return nil
}

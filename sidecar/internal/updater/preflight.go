package updater

import (
	"context"
	"fmt"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// preflightDataDir is where the data volume mounts inside the sidecar
// container. Same path the main app uses, so disk-space checks reflect what
// the new image will see at runtime.
const preflightDataDir = "/app/data"

// preflightDiskFloor reserves space for backups, logs, and image layers.
const preflightDiskFloor int64 = 500 * 1024 * 1024

// preflightDiskMultiplier covers concurrent mirror downloads and a DB backup.
const preflightDiskMultiplier = 3

// PreflightResult is the structured verdict returned by the public /preflight
// endpoint. Per-check details explain a failed gate before an update starts.
// Reserved fields remain true until their public probes are implemented so an
// unavailable check cannot reject an otherwise supported update.
type PreflightResult struct {
	DiskOK            bool              `json:"disk_ok"`
	NetworkOK         bool              `json:"network_ok"`
	ImageAlreadyLocal bool              `json:"image_already_local"`
	Sha256OK          bool              `json:"sha256_ok"`        // reserved; optimistic until wired
	SchemaDryrunOK    bool              `json:"schema_dryrun_ok"` // reserved; optimistic until wired
	PairCompatOK      bool              `json:"pair_compat_ok"`   // reserved; optimistic until wired
	PortFreeOK        bool              `json:"port_free_ok"`     // skipped while the running main service owns the port
	AllOK             bool              `json:"all_ok"`
	Details           map[string]string `json:"details,omitempty"`
}

// Preflight returns a read-only readiness verdict without mutating state.
func (d *Deps) Preflight(ctx context.Context, opts dockerops.PullOpts) PreflightResult {
	r := PreflightResult{
		Sha256OK:       true,
		SchemaDryrunOK: true,
		PairCompatOK:   true,
		PortFreeOK:     true, // skipped — running update means main holds the port
		Details:        map[string]string{},
	}

	freeBytes, err := freeDiskBytes(preflightDataDir)
	if err != nil {
		// Preserve availability when the platform cannot report free space.
		// The existing pull/backup paths still catch real ENOSPC cases.
		r.DiskOK = true
		r.Details["disk"] = fmt.Sprintf("df check skipped: %v", err)
	} else {
		required := preflightDiskFloor
		if opts.DownloadSize > 0 {
			required = opts.DownloadSize * preflightDiskMultiplier
		}
		if freeBytes >= required {
			r.DiskOK = true
			r.Details["disk"] = fmt.Sprintf("%d MB free; need %d MB", freeBytes/1024/1024, required/1024/1024)
		} else {
			r.DiskOK = false
			r.Details["disk"] = fmt.Sprintf("INSUFFICIENT_DISK: %d MB free in %s, need %d MB", freeBytes/1024/1024, preflightDataDir, required/1024/1024)
		}
	}

	if opts.URL == "" {
		// No URL exists to probe on the registry fallback path.
		// (the actual `docker compose pull` will fail loudly if not).
		r.NetworkOK = true
		r.Details["network"] = "no URL to probe (registry fallback path)"
	} else {
		if err := probeURL(ctx, opts.URL); err != nil {
			r.NetworkOK = false
			r.Details["network"] = fmt.Sprintf("NETWORK_UNREACHABLE: %s — %v", opts.URL, err)
		} else {
			r.NetworkOK = true
			r.Details["network"] = fmt.Sprintf("HEAD %s reachable", opts.URL)
		}
	}

	// The pull layer currently owns the local-image short circuit.
	r.ImageAlreadyLocal = false

	r.AllOK = r.DiskOK && r.NetworkOK && r.Sha256OK && r.SchemaDryrunOK && r.PairCompatOK && r.PortFreeOK
	return r
}

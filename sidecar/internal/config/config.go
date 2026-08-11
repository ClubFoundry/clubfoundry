// Package config persists the operator-tunable settings (autoUpdate toggle,
// update window, channel) to /app/data/updater-config.json.
//
// Reads default to the hardcoded safety-first values. Writes go through
// atomic rename. The values are exposed via GET /config and modified via
// PUT /config so the operator can change them from the main app's UI.
package config

import "errors"

// ErrValidation is returned by Validate() for any invalid input; handlers
// can use errors.Is to decide between 400 (client) and 500 (server).
var ErrValidation = errors.New("invalid config")

// Settings is the JSON shape stored on disk and returned by /config.
type Settings struct {
	// AutoUpdate allows unattended installs during UpdateWindow. It defaults
	// on for stable and is explicitly disabled when the UI changes maturity.
	AutoUpdate bool `json:"auto_update"`

	// UpdateWindow is a "HH:MM-HH:MM" range in UTC. Auto-updates only
	// trigger inside this window (to avoid disrupting the club during
	// peak hours).
	UpdateWindow string `json:"update_window"`

	// Channel selects the server-side release path.
	// Allowed: alpha, beta, stable (default), and lts.
	Channel string `json:"channel"`

	// CheckIntervalSec determines how often the sidecar polls the cloud
	// for updates. 1 hour default; lower values shorten the time between
	// release and install.
	CheckIntervalSec int `json:"check_interval_sec"`

	// AutoPruneOptOut uses opt-out semantics so a missing field keeps
	// auto-pruning enabled. The operator must explicitly disable it.
	AutoPruneOptOut bool `json:"auto_prune_opt_out"`

	// AutoPruneGraceDays protects recently published tags. Zero-valued legacy
	// files receive the one-day default. Range 1..30.
	AutoPruneGraceDays int `json:"auto_prune_grace_days"`

	// AutoPruneKeepVersions keeps extra rollback candidates after :current and
	// :previous. Zero-valued legacy files receive the default of 2. Range 1..10.
	AutoPruneKeepVersions int `json:"auto_prune_keep_versions"`

	// Build cache is separate BuildKit storage managed by `docker buildx prune`;
	// tagged-image retention does not reclaim it.

	// AutoPruneBuildCacheOptOut: opt-out flag, parallel to AutoPruneOptOut.
	// Default false (enabled).
	AutoPruneBuildCacheOptOut bool `json:"auto_prune_buildcache_opt_out"`

	// AutoPruneBuildCacheKeepGB: cap on buildkit cache size in GB. Anything
	// beyond this is reclaimed each cycle. Default 2 (filled by merge()
	// when zero). Range 1..200.
	AutoPruneBuildCacheKeepGB int `json:"auto_prune_buildcache_keep_gb"`

	// AutoPruneBuildCacheAgeDays: also evict cache entries older than this
	// many days. Default 3 (filled by merge() when zero). Range 1..30.
	AutoPruneBuildCacheAgeDays int `json:"auto_prune_buildcache_age_days"`
}

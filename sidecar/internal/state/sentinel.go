// Trampoline sentinel: how the new sidecar finalizes self-update state
// after the old sidecar exited.
//
// The old sidecar cannot observe the outcome of its own self-update because
// the trampoline recreates it with `docker compose up -d --force-recreate`.
// Process exit code is therefore not durable across the restart. The
// trampoline writes a sentinel JSON before exiting; the new sidecar reads
// it at boot and finalizes selfState.json.
//
// Sentinel directory: data/sidecar-state/recreate-status/{trampolineID}.json
// Bind-mounted into the sidecar AND the trampoline at the same path, so
// both can write/read it.
package state

import (
	"path/filepath"
)

// TrampolineSentinel is the on-disk shape the trampoline writes after
// `docker compose up -d --force-recreate` returns. The new sidecar reads
// these at boot and finalizes selfState.
//
// Layout matches the printf format in dockerops.buildTrampolineShell; keep
// both representations in sync.
type TrampolineSentinel struct {
	TrampolineID  string `json:"trampoline_id"`
	TargetVersion string `json:"target_version"`
	Service       string `json:"service"`
	// OpID correlates the sentinel with an in-flight self-update. A different
	// non-empty ID is stale and discarded. An empty ID falls back to
	// CompletedAt correlation for compatibility.
	OpID        string `json:"op_id,omitempty"`
	ExitCode    int    `json:"exit_code"`
	CompletedAt string `json:"completed_at"`
}

// SentinelPath builds the canonical path under dataDir for a given
// trampoline ID. Used by the SELF-update spawner before passing the path
// into TrampolineOpts. Empty dataDir returns "" so callers can skip
// sentinel writeback in tests.
func SentinelPath(dataDir, trampolineID string) string {
	if dataDir == "" || trampolineID == "" {
		return ""
	}
	return filepath.Join(recreateStatusDir(dataDir), trampolineID+".json")
}

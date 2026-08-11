// State persistence keeps one authoritative cross-restart file per operation
// kind. Atomic replacement protects each file from partial writes.
package state

import (
	"path/filepath"
)

// persistedState is independent from the HTTP snapshot and evolves additively.
type persistedState struct {
	Kind            Kind              `json:"kind"`
	Phase           Phase             `json:"phase"`
	SubStep         SubStep           `json:"sub_step,omitempty"`
	Detail          string            `json:"detail,omitempty"`
	LastError       ErrorInfo         `json:"last_error,omitempty"`
	StartedUnix     int64             `json:"started_unix,omitempty"`
	StartedOpUnix   int64             `json:"started_op_unix,omitempty"`
	UpdateID        string            `json:"update_id,omitempty"`
	OpID            string            `json:"op_id,omitempty"`
	TargetVersion   string            `json:"target_version,omitempty"`
	StagedTarget    string            `json:"staged_target,omitempty"`
	Step            *StepInfo         `json:"step,omitempty"`
	Download        *DownloadProgress `json:"download,omitempty"`
	CancelRequested bool              `json:"cancel_requested,omitempty"`
	// PendingMainTarget is a queued main-app target persisted across sidecar
	// recreation so the main update can resume afterward.
	PendingMainTarget string `json:"pending_main_target,omitempty"`
	// SchemaVersion rejects forward-incompatible state files.
	SchemaVersion int `json:"schema_version"`
}

const persistSchemaVersion = 1

// stateFilePath returns an empty path for memory-only states.
func stateFilePath(dataDir string, kind Kind) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "sidecar-state", string(kind)+".json")
}

// recreateStatusDir stores self-update trampoline sentinels.
func recreateStatusDir(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "sidecar-state", "recreate-status")
}

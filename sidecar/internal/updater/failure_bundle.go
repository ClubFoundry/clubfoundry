package updater

import (
	"github.com/clubfoundry/updater/internal/history"
)

// failureBundleDir is the on-disk location for per-rollback diagnostic
// bundles. Each rollback writes one file here. UI surfaces "Update X→Y
// rolled back. Send report? [Preview] [Send] [Decline]" banner on next
// operator visit, reusing the existing /report/generate pipeline.
//
// Capture is local-first and does not transmit data. Sending remains a separate
// operator-consented action.
const failureBundleDir = "update-failures"

// failureBundleMaxLogBytes caps the embedded log tail. Full log stays
// at logDir/<updateID>.log and the bundle just records the path; UI
// can fetch the full log via /log-tail when the operator hits Preview.
const failureBundleMaxLogBytes = 64 * 1024

// FailureBundle is what we serialize. Field names match the bug-report
// pipeline's JSON conventions so the eventual Send path can submit this
// bundle directly via /report/generate without a transformation step.
type FailureBundle struct {
	SchemaVersion int           `json:"schema_version"`
	UpdateID      string        `json:"update_id"`
	WrittenAt     string        `json:"written_at"`
	FromVersion   string        `json:"from_version"`
	ToVersion     string        `json:"to_version"`
	Outcome       string        `json:"outcome"`
	DurationMS    int64         `json:"duration_ms"`
	Error         string        `json:"error,omitempty"`
	ErrorCode     string        `json:"error_code,omitempty"`
	LogPath       string        `json:"log_path,omitempty"`
	LogTail       string        `json:"log_tail,omitempty"`
	Source        string        `json:"source"` // "rollback" | "auto_rollback" | "monitor_crash_loop"
	HistoryEntry  history.Entry `json:"history_entry"`
}

const failureBundleSchemaVersion = 1

// Per-update diagnostics use a flat artifact directory:
//
//	data/update-logs/{updateID}/
//	  update.log
//	  phases.jsonl
//	  state-pre.json
//	  state-post.json
//	  trampoline.{stdout,stderr}
//
// Diagnostic storage is best-effort. A logging failure must never block an
// update, so unavailable files degrade to silent no-ops.
package updater

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/clubfoundry/updater/internal/state"
)

// updateLog owns the diagnostic artifacts for one update operation. Its
// methods are nil-safe so callers do not need branches for logging failures.
type updateLog struct {
	mu       sync.Mutex
	dir      string
	updateID string
	opID     string
	kind     state.Kind
	text     *os.File
	phases   *os.File
	closed   bool
}

func openUpdateLog(rootDir, updateID, opID string, kind state.Kind) *updateLog {
	u := &updateLog{
		updateID: updateID,
		opID:     opID,
		kind:     kind,
	}
	if rootDir == "" || updateID == "" {
		return u
	}

	dir := filepath.Join(rootDir, safeFilename(updateID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return u
	}
	u.dir = dir
	migrateLegacyFlatLog(rootDir, updateID, dir)

	// Append preserves content moved from the legacy flat-log layout.
	if f, err := os.OpenFile(filepath.Join(dir, "update.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		u.text = f
	}
	if f, err := os.OpenFile(filepath.Join(dir, "phases.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		u.phases = f
	}
	return u
}

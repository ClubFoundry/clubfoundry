// Package history maintains the bounded log of past update attempts.
//
// Log is stored as a JSON array in /app/data/updater-history.json with
// atomic write-via-rename so an interrupted write can't leave the file
// corrupt. Records capture timestamps, versions, outcomes, duration, and
// errors. Pending self-update records are finalized after the new sidecar boots.
//
// Only the last 100 records are retained — older rows are pruned on each
// append. The HTTP /history endpoint returns the last N via List().
package history

import (
	"sync"
)

// Log stores a bounded, process-safe view of completed update attempts.
type Log struct {
	path string
	cap  int
	mu   sync.Mutex
}

// New returns a history log backed by path.
func New(path string) *Log {
	return &Log{path: path, cap: 100}
}

// Package recoveryhistory persists the monitor's rolling recovery event log.
package recoveryhistory

import (
	"path/filepath"
	"sync"
	"time"
)

// Kind classifies a recovery event on the sidecar wire.
type Kind string

const (
	KindRecover   Kind = "recover"
	KindReinstall Kind = "reinstall"
	KindHalt      Kind = "halt"
)

// Event records one automatic recovery action.
type Event struct {
	Kind    Kind      `json:"kind"`
	At      time.Time `json:"at"`
	Reason  string    `json:"reason"`
	Version string    `json:"version,omitempty"`
}

// EventStore is a concurrency-safe retained recovery history.
type EventStore struct {
	mu        sync.Mutex
	storePath string
	events    []Event
	persistEr error
}

const (
	maxEvents = 200
	retention = 30 * 24 * time.Hour
)

// NewStore loads the persisted history or creates an in-memory-only store.
func NewStore(dataDir string) *EventStore {
	s := &EventStore{}
	if dataDir != "" {
		s.storePath = filepath.Join(dataDir, "sidecar-state", "recovery-events.json")
		s.restoreFromDisk()
	}
	return s
}

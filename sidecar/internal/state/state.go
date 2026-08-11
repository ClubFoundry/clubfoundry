// Package state holds one durable state machine per operation kind.
//
// Main-application and sidecar self-updates use separate State instances and
// files under data/sidecar-state/{kind}.json. Within one kind, phase changes
// are explicit and guarded. Cross-kind overlap is rejected by HTTP handlers.
//
// Phase is the coarse operation state. SubStep, Step, and Download provide
// progress details without changing which phase transitions are legal.
//
// Invariants:
//   - Idle is the only state that accepts a new operation.
//   - RollingBack and Updating are mutually exclusive.
//   - Error remains visible until explicit recovery.
//   - Durable mutators persist while holding the state lock.
//   - High-frequency download samples stay in memory and notify the hook only.
package state

import "time"

// New constructs an in-memory main-operation state.
func New() *State {
	return NewKindAware(KindMain, "")
}

// NewKindAware constructs one operation state and restores its durable file.
// An empty dataDir disables persistence.
func NewKindAware(kind Kind, dataDir string) *State {
	s := &State{
		kind:      kind,
		storePath: stateFilePath(dataDir, kind),
		phase:     Idle,
		started:   time.Now(),
	}
	s.mu.Lock()
	s.restoreFromDisk()
	s.mu.Unlock()
	return s
}

// removeStateFile keeps filesystem access behind the persistence layer.
func removeStateFile(path string) error {
	return removeFileBestEffort(path)
}

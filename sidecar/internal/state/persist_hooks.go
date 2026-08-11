package state

import "fmt"

// writePersist stores the current state and synchronously notifies its observer.
// Caller must hold s.mu.
func (s *State) writePersist() {
	if s.storePath != "" {
		ps := s.snapshotPersistedLocked()
		if err := writePersistedAtomic(s.storePath, ps); err != nil {
			s.persistErr = err
		}
	}
	s.fireChangeHookLocked()
}

// fireChangeHookLocked notifies the observer without touching disk.
// Caller must hold s.mu.
func (s *State) fireChangeHookLocked() {
	if s.onChange != nil {
		hook := s.onChange
		snap := s.snapshotLocked()
		defer func() {
			if r := recover(); r != nil {
				s.persistErr = fmt.Errorf("onChange hook panicked: %v", r)
			}
		}()
		hook(snap)
	}
}

// snapshotPersistedLocked builds the durable shape. Caller must hold s.mu.
func (s *State) snapshotPersistedLocked() persistedState {
	ps := persistedState{
		Kind:              s.kind,
		Phase:             s.phase,
		SubStep:           s.subStep,
		Detail:            s.detail,
		LastError:         s.lastErr,
		UpdateID:          s.updateID,
		OpID:              s.opID,
		TargetVersion:     s.targetVer,
		StagedTarget:      s.stagedTarget,
		CancelRequested:   s.cancelRequested,
		PendingMainTarget: s.pendingMainTarget,
		SchemaVersion:     persistSchemaVersion,
	}
	if !s.started.IsZero() {
		ps.StartedUnix = s.started.Unix()
	}
	if !s.startedOp.IsZero() {
		ps.StartedOpUnix = s.startedOp.Unix()
	}
	if s.step != nil {
		stepCopy := *s.step
		ps.Step = &stepCopy
	}
	if s.download != nil {
		dlCopy := *s.download
		ps.Download = &dlCopy
	}
	return ps
}

// RegisterChangeHook replaces the single synchronous state observer.
// The callback runs while s.mu is held, so it must not call State methods.
func (s *State) RegisterChangeHook(fn func(Snapshot)) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

// GetChangeHook returns the current observer for explicit composition.
func (s *State) GetChangeHook() func(Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.onChange
}

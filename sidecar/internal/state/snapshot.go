package state

// Kind returns the immutable operation kind.
func (s *State) Kind() Kind {
	return s.kind
}

// PersistErr returns the most recent durable-write or restore error.
func (s *State) PersistErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistErr
}

// Snapshot returns an isolated copy of the current wire state.
func (s *State) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

// snapshotLocked builds the wire shape. Caller must hold s.mu.
func (s *State) snapshotLocked() Snapshot {
	snap := Snapshot{
		Kind:              s.kind,
		Phase:             s.phase,
		SubStep:           s.subStep,
		Detail:            s.detail,
		SinceEpoch:        s.started.Unix(),
		UpdateID:          s.updateID,
		OpID:              s.opID,
		TargetVersion:     s.targetVer,
		StagedTarget:      s.stagedTarget,
		CancelRequested:   s.cancelRequested,
		PendingMainTarget: s.pendingMainTarget,
	}
	if !s.startedOp.IsZero() {
		snap.StartedEpoch = s.startedOp.Unix()
	}
	if s.lastErr.Code != "" || s.lastErr.Message != "" {
		snap.LastError = s.lastErr.Message
		snap.LastErrorCode = s.lastErr.Code
	}
	if s.step != nil {
		stepCopy := *s.step
		snap.Step = &stepCopy
	}
	if s.download != nil {
		downloadCopy := *s.download
		snap.Download = &downloadCopy
	}
	return snap
}

// IsIdle reports whether this kind has no operation in flight.
func (s *State) IsIdle() bool {
	return s.Snapshot().Phase == Idle
}

// IsBusy reports whether this kind should block another operation start.
func (s *State) IsBusy() bool {
	phase := s.Snapshot().Phase
	return phase != Idle && phase != Error
}

package state

import "time"

// SetPendingMainTarget records the main target queued after self-update.
func (s *State) SetPendingMainTarget(target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingMainTarget = target
	s.writePersist()
}

// ClearPendingMainTarget clears only the queued main target.
func (s *State) ClearPendingMainTarget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingMainTarget = ""
	s.writePersist()
}

// PendingMainTarget returns the queued main target.
func (s *State) PendingMainTarget() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingMainTarget
}

// SetUpdateID records the history identifier for the current operation.
func (s *State) SetUpdateID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateID = id
	s.writePersist()
}

// SetOpID records the correlation identifier for the current operation.
func (s *State) SetOpID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opID = id
	s.writePersist()
}

// OpID returns the current operation correlation identifier.
func (s *State) OpID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opID
}

// OpStartedAt returns when the current operation entered its active phase.
func (s *State) OpStartedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startedOp
}

// SetTarget records the final target version.
func (s *State) SetTarget(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetVer = version
	s.writePersist()
}

// SetStagedTarget records the locally cached target version.
func (s *State) SetStagedTarget(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stagedTarget = version
	s.writePersist()
}

// StagedTarget returns the locally cached target version.
func (s *State) StagedTarget() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stagedTarget
}

// RequestCancel exposes the operator cancellation request in status.
func (s *State) RequestCancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelRequested = true
	s.writePersist()
}

// CancelRequested reports whether cancellation is currently requested.
func (s *State) CancelRequested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelRequested
}

// PhaseEnteredAt returns when the current phase or sub-step began.
func (s *State) PhaseEnteredAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

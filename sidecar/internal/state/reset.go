package state

import "time"

// Reset clears live operation state while preserving a queued main target.
func (s *State) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = Idle
	s.subStep = SubStepNone
	s.detail = ""
	s.lastErr = ErrorInfo{}
	s.step = nil
	s.download = nil
	s.updateID = ""
	s.opID = ""
	s.targetVer = ""
	s.startedOp = time.Time{}
	s.stagedTarget = ""
	s.cancelRequested = false
	s.started = time.Now()
	s.writePersist()
}

// ForceReset clears all state, including a queued main target.
func (s *State) ForceReset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = Idle
	s.subStep = SubStepNone
	s.detail = ""
	s.lastErr = ErrorInfo{}
	s.step = nil
	s.download = nil
	s.updateID = ""
	s.opID = ""
	s.targetVer = ""
	s.startedOp = time.Time{}
	s.stagedTarget = ""
	s.cancelRequested = false
	s.pendingMainTarget = ""
	s.started = time.Now()
	if s.storePath != "" {
		_ = removeStateFile(s.storePath)
	}
	s.writePersist()
}

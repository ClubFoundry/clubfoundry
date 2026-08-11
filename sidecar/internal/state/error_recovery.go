package state

import (
	"fmt"
	"time"
)

// AutoSoftRecoverMinDuration keeps errors visible before automatic cleanup.
const AutoSoftRecoverMinDuration = 10 * time.Minute

// MarkError records a failure while preserving progress for diagnostics.
func (s *State) MarkError(code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = Error
	s.lastErr = ErrorInfo{Code: code, Message: msg}
	s.started = time.Now()
	s.writePersist()
}

// ClearError removes the sticky error shown to the operator.
func (s *State) ClearError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = ErrorInfo{}
	s.writePersist()
}

// AutoSoftRecoverIfStuck clears stale live error state without retrying work.
// Callers must enforce any external health preconditions.
func (s *State) AutoSoftRecoverIfStuck(minDuration time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != Error {
		return false
	}
	stuckSince := time.Since(s.started)
	if stuckSince < minDuration {
		return false
	}
	s.phase = Idle
	s.subStep = SubStepNone
	s.detail = fmt.Sprintf("auto-soft-recovered after %s in Error", stuckSince.Round(time.Minute))
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
	return true
}

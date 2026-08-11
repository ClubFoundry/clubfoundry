package state

import (
	"fmt"
	"time"
)

// TransitionTo moves the state machine to an allowed phase.
func (s *State) TransitionTo(to Phase, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !legalTransition(s.phase, to) {
		return fmt.Errorf("illegal state transition: %s → %s", s.phase, to)
	}
	s.phase = to
	// Prevent cancellation state from leaking into the next operation.
	s.cancelRequested = false
	s.subStep = SubStepNone
	s.detail = detail
	s.started = time.Now()
	switch to {
	case Idle:
		s.step = nil
		s.download = nil
		s.updateID = ""
		s.opID = ""
		s.targetVer = ""
		s.startedOp = time.Time{}
		s.stagedTarget = ""
		// Completed failures belong in history, not live idle state.
		s.lastErr = ErrorInfo{}
	case Staging:
		s.lastErr = ErrorInfo{}
		s.startedOp = time.Now()
		s.step = nil
		s.download = nil
		s.stagedTarget = ""
	case Staged:
		s.step = nil
		s.download = nil
	case Updating:
		s.lastErr = ErrorInfo{}
		s.startedOp = time.Now()
		s.step = nil
		s.download = nil
	case Cancelling:
		// Preserve progress while the operation drains its context.
	case Error:
		// MarkError records the failure detail separately.
	}
	s.writePersist()
	return nil
}

// legalTransition is the complete state-transition allowlist.
func legalTransition(from, to Phase) bool {
	switch from {
	case Idle:
		return to == Checking || to == Staging || to == Updating || to == RollingBack
	case Checking:
		return to == Idle || to == Updating || to == Error
	case Staging:
		return to == Staged || to == Idle || to == Error || to == Cancelling
	case Staged:
		return to == Updating || to == Idle
	case Updating:
		return to == Idle || to == RollingBack || to == Error || to == Cancelling
	case Cancelling:
		return to == Idle || to == RollingBack || to == Error
	case RollingBack:
		return to == Idle || to == Error
	case Error:
		return to == RollingBack || to == Idle || to == Updating
	}
	return false
}

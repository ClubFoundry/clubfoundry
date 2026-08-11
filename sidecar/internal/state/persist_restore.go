package state

import (
	"fmt"
	"time"
)

// restoreFromDisk replaces in-memory fields from the per-kind state file.
// Caller must hold s.mu.
func (s *State) restoreFromDisk() {
	if s.storePath == "" {
		return
	}
	ps, err := readPersisted(s.storePath)
	if err != nil {
		s.persistErr = err
		return
	}
	if ps == nil {
		return
	}
	if ps.Kind != "" && ps.Kind != s.kind {
		s.persistErr = fmt.Errorf("state file %s has kind=%q, expected %q — refusing to import", s.storePath, ps.Kind, s.kind)
		return
	}
	s.phase = ps.Phase
	if s.phase == "" {
		s.phase = Idle
	}
	s.subStep = ps.SubStep
	s.detail = ps.Detail
	s.lastErr = ps.LastError
	if s.phase == Idle && (s.lastErr.Code != "" || s.lastErr.Message != "") {
		s.lastErr = ErrorInfo{}
	}
	s.updateID = ps.UpdateID
	s.opID = ps.OpID
	s.targetVer = ps.TargetVersion
	s.stagedTarget = ps.StagedTarget
	s.cancelRequested = ps.CancelRequested
	s.pendingMainTarget = ps.PendingMainTarget
	if ps.StartedUnix > 0 {
		s.started = time.Unix(ps.StartedUnix, 0)
	}
	if ps.StartedOpUnix > 0 {
		s.startedOp = time.Unix(ps.StartedOpUnix, 0)
	}
	if ps.Step != nil {
		stepCopy := *ps.Step
		s.step = &stepCopy
	}
	if ps.Download != nil {
		dlCopy := *ps.Download
		s.download = &dlCopy
	}
}

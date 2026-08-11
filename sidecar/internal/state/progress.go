package state

import "time"

// UpdateDetail changes the current detail without changing phase.
func (s *State) UpdateDetail(detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detail = detail
	s.writePersist()
}

// UpdateSubStep records progress and starts its watchdog deadline.
func (s *State) UpdateSubStep(sub SubStep, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subStep == SubStepDownloading && sub != SubStepDownloading {
		s.download = nil
	}
	s.subStep = sub
	s.detail = detail
	s.started = time.Now()
	s.writePersist()
}

// UpdateStep sets the stepped-update hop counter. Nil clears it.
func (s *State) UpdateStep(step *StepInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if step == nil {
		s.step = nil
		s.writePersist()
		return
	}
	stepCopy := *step
	s.step = &stepCopy
	s.writePersist()
}

// UpdateDownload publishes an in-memory sample without a durable write.
func (s *State) UpdateDownload(download DownloadProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	downloadCopy := download
	s.download = &downloadCopy
	s.fireChangeHookLocked()
}

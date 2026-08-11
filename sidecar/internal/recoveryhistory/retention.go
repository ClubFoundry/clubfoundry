package recoveryhistory

import "time"

func (s *EventStore) pruneLocked() {
	cutoff := time.Now().Add(-retention)
	kept := s.events[:0]
	for _, ev := range s.events {
		if ev.At.Before(cutoff) {
			continue
		}
		kept = append(kept, ev)
	}
	s.events = kept
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
}

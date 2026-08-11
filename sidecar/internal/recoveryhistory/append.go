package recoveryhistory

import "time"

// Append records, prunes, and persists one recovery event.
func (s *EventStore) Append(kind Kind, reason, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, Event{
		Kind:    kind,
		At:      time.Now().UTC(),
		Reason:  reason,
		Version: version,
	})
	s.pruneLocked()
	s.writePersistLocked()
}

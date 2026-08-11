package recoveryhistory

import (
	"sort"
	"time"
)

// RecentSince returns matching events sorted oldest first.
func (s *EventStore) RecentSince(cutoff time.Time) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, len(s.events))
	for _, ev := range s.events {
		if !ev.At.Before(cutoff) {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

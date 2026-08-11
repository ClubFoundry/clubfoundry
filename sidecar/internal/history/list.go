package history

import "sort"

// List returns up to limit entries in newest-first order.
func (l *Log) List(limit int) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := l.loadLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FinishedAt.After(entries[j].FinishedAt)
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

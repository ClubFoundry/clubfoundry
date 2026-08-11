package monitor

import "time"

// recordUpAttempt counts every container recreation attempt, including failures.
func (m *Monitor) recordUpAttempt() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.upAttempts = append(m.upAttempts, now)
	m.prune(now)
}

func (m *Monitor) tooManyRecentUpAttempts() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune(time.Now())
	return len(m.upAttempts) >= m.MaxRestartsInWin
}

// prune removes attempts outside the rolling restart window. Caller holds m.mu.
func (m *Monitor) prune(now time.Time) {
	cutoff := now.Add(-m.RestartWindow)
	i := 0
	for i < len(m.upAttempts) && m.upAttempts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		m.upAttempts = m.upAttempts[i:]
	}
}

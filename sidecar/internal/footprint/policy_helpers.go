package footprint

import "time"

func withAction(base PruneOutcome, action, reason string) PruneOutcome {
	base.Action = action
	base.Reason = reason
	return base
}

// imageAgeDays returns whole days or -1 when Docker's timestamp is invalid.
func imageAgeDays(createdAt string, now time.Time) int {
	const layout = "2006-01-02 15:04:05 -0700 MST"
	t, err := time.Parse(layout, createdAt)
	if err != nil {
		return -1
	}
	return int(now.Sub(t).Hours() / 24)
}

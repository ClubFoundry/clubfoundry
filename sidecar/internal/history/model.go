package history

import "time"

// Outcome describes how an update attempt ended.
type Outcome string

// Update outcomes stored in the durable operation history.
const (
	OutcomeSuccess   Outcome = "success"
	OutcomeRollback  Outcome = "rollback"
	OutcomeError     Outcome = "error"
	OutcomeCancelled Outcome = "cancelled"

	// OutcomePending records a self-update before container recreation stops
	// the writer. A later sidecar boot can finalize this uncertain outcome.
	OutcomePending Outcome = "pending"
)

// Entry records one update attempt.
type Entry struct {
	ID          string    `json:"id"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	DurationMS  int64     `json:"duration_ms"`
	FromVersion string    `json:"from_version"`
	ToVersion   string    `json:"to_version"`
	Outcome     Outcome   `json:"outcome"`
	Error       string    `json:"error,omitempty"`
	Steps       []Step    `json:"steps,omitempty"`

	// Hops lists traversed versions without FromVersion. Legacy and single-hop
	// records omit it; the frontend then falls back to ToVersion.
	Hops []string `json:"hops,omitempty"`
}

// Step describes one hop in a multi-step upgrade path.
type Step struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Outcome  Outcome `json:"outcome"`
	Duration int64   `json:"duration_ms"`
	Error    string  `json:"error,omitempty"`
}

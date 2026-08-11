package updater

import (
	"time"

	"github.com/clubfoundry/updater/internal/history"
)

// appendHistory is a nil-safe wrapper around d.History.Append.
func (d *Deps) appendHistory(e history.Entry) {
	if d.History == nil {
		return
	}
	_ = d.History.Append(e)
}

// LastSuccessfulMainUpdate returns the latest successful non-sidecar update.
func (d *Deps) LastSuccessfulMainUpdate() time.Time {
	if d.History == nil {
		return time.Time{}
	}
	entries, err := d.History.List(50)
	if err != nil {
		return time.Time{}
	}
	for _, e := range entries {
		// History is most-recent-first.
		if e.Outcome == history.OutcomeSuccess && !isSelfUpdate(e.ID) {
			return e.FinishedAt
		}
	}
	return time.Time{}
}

// isSelfUpdate identifies the per-flow ID convention: main updates use
// `upd-<unix-nano>`, self updates use `self-<unix-nano>` (selfupdate.go).
func isSelfUpdate(id string) bool {
	return len(id) >= 5 && id[:5] == "self-"
}

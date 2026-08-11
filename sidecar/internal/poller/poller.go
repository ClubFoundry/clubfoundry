// Package poller runs the background loop that periodically asks the
// ClubFoundry cloud for the latest release + recall list, then decides
// whether to trigger an auto-update or an auto-rollback.
//
// Decision tree (in order of priority):
//
//  1. Current version is in `recalled` list →
//     trigger a single-step update to resp.RollbackTo (emergency path,
//     ignores the update window).
//
//  2. Auto-update ON, current time inside update window, UpdatePath set →
//     trigger RunSteppedUpdate along the path.
//
//  3. Auto-update OFF or outside window → no-op. Main-app UI shows the
//     available-version banner (Mode A behavior even with sidecar).
package poller

import (
	"context"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/config"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

// Deps contains the services used by one automatic update polling cycle.
type Deps struct {
	Cloud   *cloud.Client
	Config  *config.Store
	Updater *updater.Deps
	State   *state.State
}

// Run blocks until ctx is cancelled. Meant to be started in a goroutine
// from main.go.
func (d *Deps) Run(ctx context.Context) {
	for {
		interval := d.interval()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			d.tick(ctx)
		}
	}
}

func (d *Deps) interval() time.Duration {
	if d.Config == nil {
		return time.Hour
	}
	set, _, _ := d.Config.Load()
	return time.Duration(set.CheckIntervalSec) * time.Second
}

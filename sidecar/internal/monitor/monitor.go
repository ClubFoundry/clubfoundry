// Package monitor applies the main-app health recovery policy.
package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
)

// RollbackTrigger restores the previous main-app image after a recent update.
type RollbackTrigger interface {
	RunRollback(ctx context.Context) error
	LastSuccessfulMainUpdate() time.Time
}

// ReinstallTrigger reloads and restarts the current main-app version.
type ReinstallTrigger interface {
	RunReinstallCurrent(ctx context.Context) error
}

// Monitor applies the configured health-recovery policy to the main service.
type Monitor struct {
	Checker   *health.Checker
	Docker    dockerops.Config
	State     *state.State
	Cloud     *cloud.Client
	Rollback  RollbackTrigger
	Reinstall ReinstallTrigger

	Events     *recoveryhistory.EventStore
	AppVersion func() string

	reinstallTriedAt time.Time

	ProbeInterval            time.Duration
	FailThreshold            int
	RestartWindow            time.Duration
	MaxRestartsInWin         int
	PostUpdateRollbackWindow time.Duration

	mu                    sync.Mutex
	upAttempts            []time.Time
	haltedAt              time.Time
	consecFails           int
	consecOks             int
	SelfState             *state.State
	AutoSoftRecoverWindow time.Duration
}

// New returns a Monitor with the production recovery thresholds.
func New(c *health.Checker, d dockerops.Config, st *state.State, cl *cloud.Client) *Monitor {
	return &Monitor{
		Checker:                  c,
		Docker:                   d,
		State:                    st,
		Cloud:                    cl,
		AutoSoftRecoverWindow:    state.AutoSoftRecoverMinDuration,
		ProbeInterval:            60 * time.Second,
		FailThreshold:            3,
		RestartWindow:            time.Hour,
		MaxRestartsInWin:         5,
		PostUpdateRollbackWindow: time.Hour,
	}
}

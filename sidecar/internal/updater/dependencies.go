// Package updater coordinates backup, image transition, health verification,
// rollback, history, and progress for one operation at a time.
package updater

import (
	"context"
	"sync"
	"time"

	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/telemetry"
)

// Deps collects injectable update collaborators and runtime configuration.
type Deps struct {
	Docker  Docker
	Backup  Backup
	Health  HealthChecker
	History *history.Log
	State   *state.State

	SelfState *state.State
	DataDir   string

	StartupWindow time.Duration
	LogDir        string
	Telemetry     *telemetry.Reporter
	SelfVersion   string
	Cloud         CloudClient

	cancelMu sync.Mutex
	cancelFn context.CancelFunc
}

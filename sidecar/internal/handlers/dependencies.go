package handlers

import (
	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/config"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

// Deps bundles the collaborators shared by HTTP handlers.
type Deps struct {
	// State tracks main application operations.
	State *state.State
	// SelfState tracks sidecar operations. Nil keeps legacy main-only callers valid.
	SelfState *state.State

	Version     string
	Updater     *updater.Deps
	History     *history.Log
	ConfigStore *config.Store
	Cloud       *cloud.Client
	LogDir      string
	// DataDir contains sidecar state, logs, and failure artifacts.
	DataDir string
	// Docker provides concrete inspection helpers required by /footprint.
	Docker dockerops.Config
	// RecoveryEvents stores automatic recovery actions exposed by /status.
	RecoveryEvents *recoveryhistory.EventStore
}

package bootstrap

import (
	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/state"
)

// Deps contains the collaborators and operator-selected bootstrap settings.
type Deps struct {
	Docker      dockerops.Config
	Cloud       *cloud.Client
	State       *state.State
	Channel     string
	SelfVersion string
	HealthURL   string
}

// healthURLOrDefault shares the port-aware main-app resolver used elsewhere.
func (d Deps) healthURLOrDefault() string {
	if d.HealthURL != "" {
		return d.HealthURL
	}
	return health.ResolveMainHealthURL()
}

package monitor

import (
	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
)

// SetRollbackTrigger configures optional automatic rollback.
func (m *Monitor) SetRollbackTrigger(rt RollbackTrigger) {
	m.Rollback = rt
}

// SetReinstallTrigger configures optional same-version recovery.
func (m *Monitor) SetReinstallTrigger(rt ReinstallTrigger) {
	m.Reinstall = rt
}

// SetEventStore configures persistent operator-facing recovery events.
func (m *Monitor) SetEventStore(es *recoveryhistory.EventStore) {
	m.Events = es
}

// SetSelfState includes self-update errors in stale-state recovery.
func (m *Monitor) SetSelfState(ss *state.State) {
	m.SelfState = ss
}

// SetAppVersionGetter provides version context for recovery events.
func (m *Monitor) SetAppVersionGetter(fn func() string) {
	m.AppVersion = fn
}

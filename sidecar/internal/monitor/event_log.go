package monitor

import "github.com/clubfoundry/updater/internal/recoveryhistory"

func (m *Monitor) recordEvent(kind recoveryhistory.Kind, reason string) {
	if m.Events == nil {
		return
	}
	v := ""
	if m.AppVersion != nil {
		v = m.AppVersion()
	}
	m.Events.Append(kind, reason, v)
}

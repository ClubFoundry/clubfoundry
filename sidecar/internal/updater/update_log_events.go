package updater

import (
	"encoding/json"
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

const phaseEventSchemaVersion = 1

type phaseEvent struct {
	SchemaVersion int            `json:"schema_version"`
	TS            string         `json:"ts"`
	UpdateID      string         `json:"update_id"`
	OpID          string         `json:"op_id,omitempty"`
	Kind          string         `json:"kind,omitempty"`
	Phase         string         `json:"phase"`
	SubStep       string         `json:"sub_step,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	LastErrorCode string         `json:"last_error_code,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	Extras        map[string]any `json:"extras,omitempty"`
}

func (u *updateLog) hookFn() func(state.Snapshot) {
	if u == nil {
		return func(state.Snapshot) {}
	}
	return func(snap state.Snapshot) {
		u.appendPhaseFromSnapshot(snap, nil)
	}
}

func (u *updateLog) appendPhaseFromSnapshot(snap state.Snapshot, extras map[string]any) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.phases == nil || u.closed {
		return
	}
	event := phaseEvent{
		SchemaVersion: phaseEventSchemaVersion,
		TS:            time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		UpdateID:      u.updateID,
		OpID:          u.opID,
		Kind:          string(u.kind),
		Phase:         string(snap.Phase),
		SubStep:       string(snap.SubStep),
		Detail:        snap.Detail,
		LastErrorCode: snap.LastErrorCode,
		LastError:     snap.LastError,
		Extras:        extras,
	}
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	body = append(body, '\n')
	_, _ = u.phases.Write(body)
}

func (u *updateLog) appendPhaseExtras(phase state.Phase, sub state.SubStep, detail string, extras map[string]any) {
	u.appendPhaseFromSnapshot(state.Snapshot{Phase: phase, SubStep: sub, Detail: detail}, extras)
}

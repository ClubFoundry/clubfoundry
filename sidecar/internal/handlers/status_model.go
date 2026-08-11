package handlers

import (
	"time"

	"github.com/clubfoundry/updater/internal/recoveryhistory"
	"github.com/clubfoundry/updater/internal/state"
)

// StatusResponse preserves legacy top-level fields and adds kind-scoped state.
type StatusResponse struct {
	state.Snapshot
	MainOp            *state.Snapshot         `json:"main_op,omitempty"`
	SelfOp            *state.Snapshot         `json:"self_op,omitempty"`
	RecoveryEvents24h []recoveryhistory.Event `json:"recovery_events_24h,omitempty"`
}

// composeStatus mirrors the active operation into legacy top-level fields.
func composeStatus(main, self *state.State, events *recoveryhistory.EventStore) StatusResponse {
	mainSnap := main.Snapshot()
	resp := StatusResponse{Snapshot: mainSnap, MainOp: &mainSnap}
	if self != nil {
		selfSnap := self.Snapshot()
		resp.SelfOp = &selfSnap
		// Main wins ties because concurrent starts are rejected at entry.
		if mainSnap.Phase == state.Idle && selfSnap.Phase != state.Idle {
			resp.Snapshot = selfSnap
		}
	}
	if events != nil {
		resp.RecoveryEvents24h = events.RecentSince(time.Now().Add(-24 * time.Hour))
	}
	return resp
}

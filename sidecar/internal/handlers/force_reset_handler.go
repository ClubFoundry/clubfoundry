package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/clubfoundry/updater/internal/state"
)

func registerForceResetHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/force-reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		kindParam := strings.TrimSpace(r.URL.Query().Get("kind"))
		var target *state.State
		switch kindParam {
		case string(state.KindMain):
			target = d.State
		case string(state.KindSelf):
			target = d.SelfState
		default:
			writeError(w, http.StatusBadRequest, "must specify ?kind=main or ?kind=self (force-reset is per-kind)")
			return
		}
		if target == nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("kind=%s state not initialized", kindParam))
			return
		}

		previous := target.Snapshot()
		log.Printf("force-reset: kind=%s pre-state phase=%s sub_step=%s op_id=%s target=%s — operator escape",
			target.Kind(), previous.Phase, previous.SubStep, previous.OpID, previous.TargetVersion)
		target.ForceReset()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "force_reset",
			"kind":       string(target.Kind()),
			"prev_phase": string(previous.Phase),
		})
	})
}

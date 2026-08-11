package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/clubfoundry/updater/internal/state"
)

func registerResetErrorHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/reset-error", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		kindParam := strings.TrimSpace(r.URL.Query().Get("kind"))
		var targets []*state.State
		switch kindParam {
		case "":
			if d.State != nil && d.State.Snapshot().Phase == state.Error {
				targets = append(targets, d.State)
			}
			if d.SelfState != nil && d.SelfState.Snapshot().Phase == state.Error {
				targets = append(targets, d.SelfState)
			}
			if len(targets) == 0 {
				writeError(w, http.StatusConflict, "no kind currently in error state")
				return
			}
		case string(state.KindMain):
			if d.State == nil {
				writeError(w, http.StatusServiceUnavailable, "main state not initialized")
				return
			}
			if d.State.Snapshot().Phase != state.Error {
				writeError(w, http.StatusConflict, fmt.Sprintf("kind=main not in error (phase=%s)", d.State.Snapshot().Phase))
				return
			}
			targets = []*state.State{d.State}
		case string(state.KindSelf):
			if d.SelfState == nil {
				writeError(w, http.StatusServiceUnavailable, "self state not initialized")
				return
			}
			if d.SelfState.Snapshot().Phase != state.Error {
				writeError(w, http.StatusConflict, fmt.Sprintf("kind=self not in error (phase=%s)", d.SelfState.Snapshot().Phase))
				return
			}
			targets = []*state.State{d.SelfState}
		default:
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid kind=%q (must be main or self)", kindParam))
			return
		}

		resetKinds := make([]string, 0, len(targets))
		for _, target := range targets {
			target.Reset()
			resetKinds = append(resetKinds, string(target.Kind()))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "reset",
			"kinds":  resetKinds,
		})
	})
}

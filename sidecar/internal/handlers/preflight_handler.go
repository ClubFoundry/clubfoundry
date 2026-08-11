package handlers

import (
	"encoding/json"
	"net/http"
)

// registerPreflightHandler exposes non-destructive update readiness checks.
func registerPreflightHandler(mux *http.ServeMux, d Deps) {
	// The optional target follows the same resolution rules as /update.
	mux.HandleFunc("/preflight", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		if d.Updater == nil {
			writeError(w, http.StatusServiceUnavailable, "updater not initialized")
			return
		}
		var body struct {
			Target string `json:"target"`
		}
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
				return
			}
		}
		dispatch, decideErr := resolveUpdateDispatch(r.Context(), d, body.Target)
		if decideErr != nil {
			writeError(w, http.StatusBadRequest, decideErr.Error())
			return
		}
		// Preflight probes artifact readiness without starting the path.
		result := d.Updater.Preflight(r.Context(), dispatch.MainPull)
		writeJSON(w, http.StatusOK, result)
	})
}

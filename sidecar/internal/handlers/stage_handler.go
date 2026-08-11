package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// registerStageHandler loads an image without replacing the running container.
func registerStageHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/update/stage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		if rejectIfCatalogManaged(w) {
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
		if busyKind, busy := anyBusy(d.State, d.SelfState); busy {
			writeError(w, http.StatusConflict, fmt.Sprintf("busy: kind=%s", busyKind))
			return
		}
		// Staging accepts only a direct target, not a multi-hop path.
		dispatch, decideErr := resolveUpdateDispatch(r.Context(), d, body.Target)
		if decideErr != nil {
			writeError(w, http.StatusBadRequest, decideErr.Error())
			return
		}
		if len(dispatch.Path) > 1 {
			writeError(w, http.StatusBadRequest, "stepped paths cannot be staged — call /update for the multi-hop case")
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if err := d.Updater.Stage(ctx, dispatch.Target, dispatch.MainPull); err != nil {
				log.Printf("/update/stage async path failed: target=%s err=%v", dispatch.Target, err)
			}
		}()
		resp := map[string]any{"status": "accepted", "target": dispatch.Target}
		if dispatch.MainPull.URL != "" {
			resp["artifact_url"] = dispatch.MainPull.URL
		}
		writeJSON(w, http.StatusAccepted, resp)
	})
}

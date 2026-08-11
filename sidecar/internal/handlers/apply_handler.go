package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

// registerApplyHandler installs a complete staged target.
func registerApplyHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/update/apply", func(w http.ResponseWriter, r *http.Request) {
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
		snap := d.State.Snapshot()
		if snap.Phase != state.Staged {
			writeError(w, http.StatusConflict, fmt.Sprintf("not staged (phase=%s)", snap.Phase))
			return
		}
		if snap.StagedTarget == "" {
			writeError(w, http.StatusConflict, "staged target version unknown")
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if err := d.Updater.Apply(ctx); err != nil {
				log.Printf("/update/apply async path failed: target=%s err=%v", snap.StagedTarget, err)
			}
		}()
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "accepted",
			"target": snap.StagedTarget,
		})
	})
}

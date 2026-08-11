package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

// registerRollbackHandler exposes operator-requested rollback.
func registerRollbackHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/rollback", func(w http.ResponseWriter, r *http.Request) {
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
		if snap := d.State.Snapshot(); snap.Phase == state.Updating || snap.Phase == state.RollingBack {
			writeError(w, http.StatusConflict, fmt.Sprintf("busy: %s", snap.Phase))
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := d.Updater.RunRollback(ctx); err != nil {
				log.Printf("/rollback async path failed: err=%v", err)
			}
		}()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	})
}

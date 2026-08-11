package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// registerSelfUpdateHandler exposes the sidecar self-update operation.
func registerSelfUpdateHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/self-update", func(w http.ResponseWriter, r *http.Request) {
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
		if busyKind, busy := anyBusy(d.State, d.SelfState); busy {
			writeError(w, http.StatusConflict, fmt.Sprintf("busy: kind=%s", busyKind))
			return
		}
		// Resolve artifact metadata before accepting the operation.
		// Air-gapped installs fall back to registry pull metadata.
		updaterPull, targetVer := resolveSelfUpdateDispatch(r.Context(), d)
		// The target reaches a generated shell command, so reject shell metacharacters.
		if targetVer != "" && !isSafeVersionToken(targetVer) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("rejected target %q — must match [a-zA-Z0-9._\\-:]+", targetVer))
			return
		}
		// Reject stale UI retries that would record a no-op replacement.
		if d.Version != "" && targetVer != "" && targetVer == d.Version {
			writeError(w, http.StatusConflict, fmt.Sprintf("already on target version %s", targetVer))
			return
		}
		// Explicit operator actions may downgrade during a channel switch.
		// Automated chains enforce strictly newer sidecar versions separately.
		body := map[string]string{"status": "accepted"}
		if updaterPull.URL != "" {
			body["artifact_url"] = updaterPull.URL
			body["target"] = targetVer
		}
		writeJSON(w, http.StatusAccepted, body)
		// A background context survives the accepted HTTP request.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := d.Updater.RunSelfUpdate(ctx, targetVer, updaterPull); err != nil {
				log.Printf("/self-update direct async path failed: target=%s err=%v", targetVer, err)
			}
		}()
	})

}

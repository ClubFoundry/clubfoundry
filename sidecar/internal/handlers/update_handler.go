package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/updater"
)

func registerUpdateHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
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
			// SkipChain forces a main-only update instead of upgrading sidecar first.
			SkipChain bool `json:"skip_chain,omitempty"`
		}
		// An empty body selects the latest available version.
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
				return
			}
		}

		// Reject overlap before accepting a background operation.
		if busyKind, busy := anyBusy(d.State, d.SelfState); busy {
			writeError(w, http.StatusConflict, fmt.Sprintf("busy: kind=%s", busyKind))
			return
		}

		// Resolve both the step path and the immutable artifact metadata.
		dispatch, decideErr := resolveUpdateDispatch(r.Context(), d, body.Target)
		if decideErr != nil {
			writeError(w, http.StatusBadRequest, decideErr.Error())
			return
		}

		// Single-hop updates may upgrade sidecar first and resume the main target.
		// Stepped paths own their intermediate hops and never enter this chain.
		if !body.SkipChain && d.SelfState != nil && len(dispatch.Path) == 0 {
			selfPull, selfTarget := resolveSelfUpdateDispatch(r.Context(), d)
			if updater.IsStrictlyNewerSidecarVersion(d.Version, selfTarget) && (selfPull.URL != "" || selfPull.Sha256 != "") {
				log.Printf("update: chaining self-update (%s → %s) before main (%s → %s)",
					d.Version, selfTarget, d.Updater.CurrentVersion(r.Context()), dispatch.Target)
				// The replacement sidecar resumes this target after restart.
				if d.State != nil {
					d.State.SetPendingMainTarget(dispatch.Target)
				}
				// Return before the trampoline replaces this process.
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
					defer cancel()
					if err := d.Updater.RunSelfUpdate(ctx, selfTarget, selfPull); err != nil {
						// Prevent the next request from replaying a failed chain.
						log.Printf("chained self-update failed (pre-trampoline): selfTarget=%s mainTarget=%s err=%v — clearing queued main target", selfTarget, dispatch.Target, err)
						if d.State != nil {
							d.State.ClearPendingMainTarget()
						}
					}
				}()
				resp := map[string]any{
					"status":            "accepted",
					"target":            dispatch.Target,
					"chained_self":      true,
					"self_target":       selfTarget,
					"self_artifact_url": selfPull.URL,
				}
				if dispatch.MainPull.URL != "" {
					resp["artifact_url"] = dispatch.MainPull.URL
				}
				writeJSON(w, http.StatusAccepted, resp)
				return
			}
		}

		// The operation must outlive the request that receives HTTP 202.
		go func() {
			// Stepped updates need a larger deadline for intermediate hops.
			timeout := 30 * time.Minute
			if len(dispatch.Path) > 1 {
				timeout = 90 * time.Minute
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			var err error
			if len(dispatch.Path) > 0 {
				err = d.Updater.RunSteppedUpdate(ctx, dispatch.Path)
			} else {
				err = d.Updater.RunUpdate(ctx, dispatch.Target, dispatch.MainPull)
			}
			// Keep asynchronous failures visible outside per-update artifacts.
			if err != nil {
				log.Printf("/update async path failed: target=%s steps=%d err=%v", dispatch.Target, len(dispatch.Path), err)
			}
		}()
		resp := map[string]any{
			"status": "accepted",
			"target": dispatch.Target,
		}
		if len(dispatch.Path) > 0 {
			resp["path"] = dispatch.Path
			resp["steps"] = len(dispatch.Path)
		}
		if dispatch.MainPull.URL != "" {
			resp["artifact_url"] = dispatch.MainPull.URL
		}
		writeJSON(w, http.StatusAccepted, resp)
	})
}

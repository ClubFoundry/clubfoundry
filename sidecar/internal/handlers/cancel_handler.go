package handlers

import "net/http"

// registerCancelHandler signals the operation that owns the active cancel function.
func registerCancelHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		if d.Updater == nil {
			writeError(w, http.StatusServiceUnavailable, "updater not initialized")
			return
		}
		if !d.Updater.Cancel() {
			writeError(w, http.StatusConflict, "no in-flight operation to cancel")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancel_requested"})
	})
}

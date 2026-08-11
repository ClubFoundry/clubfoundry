package handlers

import "net/http"

// registerDropStagedHandler abandons staged state without pruning the image.
func registerDropStagedHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/update/staged", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "DELETE only")
			return
		}
		if d.Updater == nil {
			writeError(w, http.StatusServiceUnavailable, "updater not initialized")
			return
		}
		if err := d.Updater.DropStaged(); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
	})
}

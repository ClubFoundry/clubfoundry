package handlers

import "net/http"

func registerStatusHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		writeJSON(w, http.StatusOK, composeStatus(d.State, d.SelfState, d.RecoveryEvents))
	})
}

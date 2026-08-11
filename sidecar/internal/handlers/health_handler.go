package handlers

import "net/http"

func registerHealthHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": d.Version,
		})
	})
}

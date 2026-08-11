package handlers

import (
	"net/http"

	"github.com/clubfoundry/updater/internal/history"
)

func registerHistoryHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		if d.History == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		entries, err := d.History.List(10)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if entries == nil {
			entries = []history.Entry{}
		}
		writeJSON(w, http.StatusOK, entries)
	})
}

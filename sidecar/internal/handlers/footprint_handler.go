package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/footprint"
)

func registerFootprintHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/footprint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		repos := []string{d.Docker.MainServiceName(), d.Docker.UpdaterServiceName()}
		filtered := repos[:0]
		for _, repo := range repos {
			if repo != "" {
				filtered = append(filtered, repo)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, footprint.Collect(ctx, d.Docker, filtered))
	})
}

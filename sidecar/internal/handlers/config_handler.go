package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/clubfoundry/updater/internal/config"
)

// registerConfigHandler owns the persisted operator-settings HTTP contract.
func registerConfigHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if d.ConfigStore == nil {
				writeJSON(w, http.StatusOK, config.Defaults())
				return
			}
			set, _, err := d.ConfigStore.Load()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, set)

		case http.MethodPut:
			if d.ConfigStore == nil {
				writeError(w, http.StatusServiceUnavailable, "config not initialized")
				return
			}
			var set config.Settings
			if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
			if err := d.ConfigStore.Save(set); err != nil {
				if errors.Is(err, config.ErrValidation) {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, set)

		default:
			writeError(w, http.StatusMethodNotAllowed, "GET or PUT")
		}
	})
}

package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func registerFailureBundleItemHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/failure-bundles/", func(w http.ResponseWriter, r *http.Request) {
		filename := strings.TrimPrefix(r.URL.Path, "/failure-bundles/")
		if filename == "" {
			writeError(w, http.StatusBadRequest, "missing filename")
			return
		}
		if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
			writeError(w, http.StatusBadRequest, "invalid filename")
			return
		}
		if filepath.Ext(filename) != ".json" {
			writeError(w, http.StatusBadRequest, "must end in .json")
			return
		}
		dir := failureBundleDirFromLogDir(d.LogDir)
		if dir == "" {
			writeError(w, http.StatusServiceUnavailable, "log dir not configured")
			return
		}
		path := filepath.Join(dir, filename)
		switch r.Method {
		case http.MethodGet:
			body, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					writeError(w, http.StatusNotFound, "bundle not found")
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case http.MethodDelete:
			if err := os.Remove(path); err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusOK, map[string]string{"status": "already_gone"})
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			writeError(w, http.StatusMethodNotAllowed, "GET or DELETE")
		}
	})
}

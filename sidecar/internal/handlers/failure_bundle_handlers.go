package handlers

import (
	"net/http"
	"path/filepath"
)

func failureBundleDirFromLogDir(logDir string) string {
	if logDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(logDir), "update-failures")
}

func registerFailureBundleHandlers(mux *http.ServeMux, d Deps) {
	registerFailureBundleListHandler(mux, d)
	registerFailureBundleItemHandler(mux, d)
}

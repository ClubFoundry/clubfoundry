package handlers

import "net/http"

// registerArtifactHandlers exposes persisted update artifacts.
func registerArtifactHandlers(mux *http.ServeMux, d Deps) {
	registerFailureBundleHandlers(mux, d)
	registerHistoryHandler(mux, d)
	registerLogTailHandler(mux, d)
}

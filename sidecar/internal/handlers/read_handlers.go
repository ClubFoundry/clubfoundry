package handlers

import "net/http"

// registerReadHandlers exposes sidecar health and diagnostic state.
func registerReadHandlers(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/diagnostic-bundle", handleDiagnosticBundle(d.DataDir, d.Version))
	registerFootprintHandler(mux, d)
	registerHealthHandler(mux, d)
	registerStatusHandler(mux, d)
}

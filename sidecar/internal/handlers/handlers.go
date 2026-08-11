// Package handlers defines and registers the sidecar HTTP API.
package handlers

import "net/http"

// Register attaches all sidecar HTTP handlers to mux.
func Register(mux *http.ServeMux, d Deps) {
	registerReadHandlers(mux, d)

	registerMainUpdateHandlers(mux, d)
	registerSelfUpdateHandler(mux, d)
	registerPreflightHandler(mux, d)
	registerRecoveryHandlers(mux, d)

	registerRollbackHandler(mux, d)
	registerArtifactHandlers(mux, d)
	registerConfigHandler(mux, d)
}

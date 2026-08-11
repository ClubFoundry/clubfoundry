package handlers

import (
	"net/http"
)

// registerRecoveryHandlers owns operator-driven state-machine recovery.
func registerRecoveryHandlers(mux *http.ServeMux, d Deps) {
	registerResetErrorHandler(mux, d)
	registerForceResetHandler(mux, d)
}

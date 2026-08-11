package handlers

import "net/http"

// registerMainUpdateHandlers exposes the main application update lifecycle.
func registerMainUpdateHandlers(mux *http.ServeMux, d Deps) {
	registerUpdateHandler(mux, d)
	registerStageHandler(mux, d)
	registerApplyHandler(mux, d)
	registerDropStagedHandler(mux, d)
	registerCancelHandler(mux, d)
}

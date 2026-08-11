package handlers

import (
	"net/http"
	"os"
	"strings"
)

// Catalog installations delegate image lifecycle management to TrueNAS Apps.
func catalogManaged() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("CLM_UPDATE_MODE")), "truenas_apps")
}

func rejectIfCatalogManaged(w http.ResponseWriter) bool {
	if !catalogManaged() {
		return false
	}
	writeJSON(w, http.StatusConflict, map[string]string{
		"error": "catalog-managed install — image lifecycle is owned by TrueNAS Apps",
		"code":  "CATALOG_MANAGED",
	})
	return true
}

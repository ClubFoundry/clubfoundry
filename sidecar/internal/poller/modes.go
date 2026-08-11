package poller

import (
	"os"
	"strings"
	"sync"
)

// Log disabled background modes once per process to avoid hourly noise.
var (
	frozenLogOnce         sync.Once
	catalogManagedLogOnce sync.Once
)

// sidecarIsFrozen reports the explicit version used to park background updates.
func sidecarIsFrozen() (bool, string) {
	v := strings.TrimSpace(os.Getenv("CLM_SIDECAR_FROZEN_VERSION"))
	return v != "", v
}

// catalogManaged reports whether TrueNAS Apps owns the image lifecycle.
func catalogManaged() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("CLM_UPDATE_MODE")), "truenas_apps")
}

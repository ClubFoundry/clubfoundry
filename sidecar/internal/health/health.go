// Package health checks main application readiness after starts and updates.
// Startup responses may expose migration progress before final health.
package health

import (
	"net/http"
	"path/filepath"
	"time"
)

// Checker probes the main application and tracks its durable boot state.
type Checker struct {
	URL           string
	Client        *http.Client
	BootStatePath string // /app/data/.boot-state.json by default
}

// DefaultChecker returns a checker configured for the resolved main health URL
// and the standard boot-state location.
func DefaultChecker() *Checker {
	dataDir := dataDirOrDefault()
	return &Checker{
		URL:           ResolveMainHealthURL(),
		Client:        &http.Client{Timeout: 10 * time.Second},
		BootStatePath: filepath.Join(dataDir, ".boot-state.json"),
	}
}

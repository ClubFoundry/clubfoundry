package handlers

import "github.com/clubfoundry/updater/internal/dockerops"

// updateDispatch contains the resolved single-hop or stepped update request.
type updateDispatch struct {
	// Path is nil for single-hop updates and ends with Target otherwise.
	Path   []string
	Target string

	// MainPull is empty when the registry fallback should be used.
	MainPull dockerops.PullOpts
}

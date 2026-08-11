package handlers

import "github.com/clubfoundry/updater/internal/state"

// anyBusy prevents main and sidecar operations from overlapping.
func anyBusy(main, self *state.State) (busyKind state.Kind, isBusy bool) {
	if main != nil && main.IsBusy() {
		return state.KindMain, true
	}
	if self != nil && self.IsBusy() {
		return state.KindSelf, true
	}
	return "", false
}

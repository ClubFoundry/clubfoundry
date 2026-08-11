package updater

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/clubfoundry/updater/internal/state"
)

// writeStateSnapshot writes atomically so readers never observe partial JSON.
func (u *updateLog) writeStateSnapshot(name string, snap state.Snapshot) {
	if u == nil || u.dir == "" || name == "" {
		return
	}
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(u.dir, name+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

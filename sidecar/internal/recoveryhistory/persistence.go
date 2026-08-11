package recoveryhistory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type persistedFile struct {
	SchemaVersion int     `json:"schema_version"`
	Events        []Event `json:"events"`
}

const persistSchemaVersion = 1

func (s *EventStore) writePersistLocked() {
	if s.storePath == "" {
		return
	}
	body, err := json.MarshalIndent(persistedFile{
		SchemaVersion: persistSchemaVersion,
		Events:        s.events,
	}, "", "  ")
	if err != nil {
		s.persistEr = fmt.Errorf("marshal recovery events: %w", err)
		return
	}
	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.persistEr = fmt.Errorf("mkdir %s: %w", dir, err)
		return
	}
	tmp := s.storePath + ".tmp"
	if err := writeFileSync(tmp, body); err != nil {
		s.persistEr = fmt.Errorf("write tmp: %w", err)
		return
	}
	if err := os.Rename(tmp, s.storePath); err != nil {
		s.persistEr = fmt.Errorf("rename: %w", err)
		return
	}
	if err := fsyncDir(dir); err != nil {
		s.persistEr = fmt.Errorf("fsync dir: %w", err)
	}
}

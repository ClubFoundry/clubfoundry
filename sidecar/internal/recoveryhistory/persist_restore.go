package recoveryhistory

import (
	"encoding/json"
	"fmt"
	"os"
)

func (s *EventStore) restoreFromDisk() {
	if s.storePath == "" {
		return
	}
	body, err := os.ReadFile(s.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		s.persistEr = fmt.Errorf("read %s: %w", s.storePath, err)
		return
	}
	var pf persistedFile
	if err := json.Unmarshal(body, &pf); err != nil {
		s.persistEr = fmt.Errorf("unmarshal recovery events: %w", err)
		return
	}
	if pf.SchemaVersion > persistSchemaVersion {
		s.persistEr = fmt.Errorf("recovery-events schema_version=%d > supported %d (sidecar downgrade not supported)", pf.SchemaVersion, persistSchemaVersion)
		return
	}
	s.events = pf.Events
	s.pruneLocked()
}

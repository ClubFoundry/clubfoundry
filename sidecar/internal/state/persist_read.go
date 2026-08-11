package state

import (
	"encoding/json"
	"fmt"
	"os"
)

func readPersisted(path string) (*persistedState, error) {
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state file %s: %w", path, err)
	}
	var ps persistedState
	if err := json.Unmarshal(body, &ps); err != nil {
		return nil, fmt.Errorf("unmarshal state file %s: %w", path, err)
	}
	if ps.SchemaVersion > persistSchemaVersion {
		return nil, fmt.Errorf("state file schema_version=%d > supported %d (sidecar downgrade not supported)", ps.SchemaVersion, persistSchemaVersion)
	}
	return &ps, nil
}

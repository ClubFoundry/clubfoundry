package history

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (l *Log) loadLocked() ([]Entry, error) {
	raw, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		// Preserve availability by replacing an unreadable history on append.
		return nil, nil
	}
	return entries, nil
}

func (l *Log) saveLocked(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp := l.path + ".part"
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

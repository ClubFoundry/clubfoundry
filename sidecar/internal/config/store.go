package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store serializes access to one settings file.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore returns a settings store backed by path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads the file and returns defaults when it does not exist.
// The boolean reports whether settings were loaded from disk.
func (s *Store) Load() (Settings, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), false, nil
		}
		return Settings{}, false, err
	}
	var set Settings
	if err := json.Unmarshal(raw, &set); err != nil {
		return Defaults(), false, fmt.Errorf("parse config: %w", err)
	}
	return merge(set), true, nil
}

// Save validates settings and replaces the file through an atomic rename.
func (s *Store) Save(in Settings) error {
	if err := Validate(in); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".part"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

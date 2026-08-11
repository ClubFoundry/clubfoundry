package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func writePersistedAtomic(path string, ps persistedState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir state dir %s: %w", dir, err)
	}
	body, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := writeFileSyncAtomic(tmp, body, 0o644); err != nil {
		return fmt.Errorf("write tmp state file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp -> final: %w", err)
	}
	if err := fsyncDir(dir); err != nil {
		return fmt.Errorf("fsync state dir %s: %w", dir, err)
	}
	return nil
}

func writeFileSyncAtomic(path string, body []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func removeFileBestEffort(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func copyDirShallow(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("mkdir dst: %w", err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("readdir src: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFileAtomic(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return fmt.Errorf("copy %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func copyFileAtomic(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer in.Close()
	tmp := destination + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy bytes: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	if dir, err := os.Open(filepath.Dir(destination)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

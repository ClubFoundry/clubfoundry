package bootstrap

import (
	"os"
	"path/filepath"
)

// WriteEnvTemplateIfMissing creates the setup placeholder without overwriting operator values.
func WriteEnvTemplateIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, []byte(envTemplate), 0o600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

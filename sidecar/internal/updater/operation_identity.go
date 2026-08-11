package updater

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
)

func trampolineLogPaths(logDir string) (string, string) {
	if logDir == "" {
		return "", ""
	}
	return filepath.Join(logDir, "trampoline.stdout"), filepath.Join(logDir, "trampoline.stderr")
}

func newOpID() string {
	return randomID("op-", 16)
}

func newTrampolineID() string {
	return randomID("tramp-", 12)
}

func randomID(prefix string, size int) string {
	value := make([]byte, size)
	_, _ = rand.Read(value)
	return prefix + hex.EncodeToString(value)
}

package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var sqliteMagic = []byte{
	0x53, 0x51, 0x4c, 0x69, 0x74, 0x65, 0x20, 0x66,
	0x6f, 0x72, 0x6d, 0x61, 0x74, 0x20, 0x33, 0x00,
}

// validateSQLiteHeader rejects non-database and truncated restore sources.
func validateSQLiteHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open for header check: %w", err)
	}
	defer f.Close()
	buf := make([]byte, 16)
	n, err := io.ReadFull(f, buf)
	if err != nil {
		return fmt.Errorf("read header (%d bytes): %w", n, err)
	}
	for i, b := range sqliteMagic {
		if buf[i] != b {
			return fmt.Errorf("not a SQLite database — header bytes %x (expected %x)", buf, sqliteMagic)
		}
	}
	return nil
}

// isMainBackupName excludes WAL, SHM, and partial siblings.
func isMainBackupName(name string) bool {
	if !strings.HasPrefix(name, "clm.db.pre-update-") {
		return false
	}
	if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
		return false
	}
	if strings.HasSuffix(name, ".part") {
		return false
	}
	return true
}

// VersionFromPath returns the source version encoded by CreateBackup.
func VersionFromPath(path string) (string, error) {
	const prefix = "clm.db.pre-update-v"
	name := filepath.Base(path)
	if !strings.HasPrefix(name, prefix) {
		return "", fmt.Errorf("not a ClubFoundry pre-update backup: %s", name)
	}
	remainder := strings.TrimPrefix(name, prefix)
	separator := strings.LastIndex(remainder, "-")
	if separator <= 0 || separator == len(remainder)-1 {
		return "", fmt.Errorf("backup name has no version or timestamp: %s", name)
	}
	version := remainder[:separator]
	stamp := remainder[separator+1:]
	if _, err := time.Parse("20060102T150405Z", stamp); err != nil {
		return "", fmt.Errorf("backup name has invalid timestamp %q: %w", stamp, err)
	}
	return version, nil
}

// sanitize replaces characters that are unsafe in backup filenames.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

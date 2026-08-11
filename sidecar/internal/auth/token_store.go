package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	tokenFileName = "auth-token"
	tokenStateDir = "sidecar-state"
	tokenLength   = 32
	// The root sidecar writes these paths; the UID 100 backend must read them.
	fileMode = 0o644
	dirMode  = 0o755
)

// Init loads a valid token or creates and atomically persists a new one.
func Init(dataDir string) (*Token, error) {
	if dataDir == "" {
		log.Printf("auth: no data dir — running in NO-AUTH mode (legacy/test only)")
		return &Token{}, nil
	}
	dir := filepath.Join(dataDir, tokenStateDir)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("auth: mkdir %s: %w", dir, err)
	}
	// Heal legacy directories that blocked traversal by the backend container.
	if err := os.Chmod(dir, dirMode); err != nil {
		log.Printf("auth: chmod %s to %o failed (continuing): %v", dir, dirMode, err)
	}
	path := filepath.Join(dir, tokenFileName)

	if data, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(data))
		if isValidTokenFormat(s) {
			// Heal legacy files whose private mode blocked backend reads.
			if err := os.Chmod(path, fileMode); err != nil {
				log.Printf("auth: chmod %s to %o failed (continuing): %v", path, fileMode, err)
			}
			log.Printf("auth: loaded token from %s", path)
			return &Token{value: s}, nil
		}
		log.Printf("auth: token file %s malformed (len=%d) — regenerating", path, len(s))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("auth: read %s: %w", path, err)
	}

	buf := make([]byte, tokenLength)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("auth: rand.Read: %w", err)
	}
	s := hex.EncodeToString(buf)
	if err := writeAtomic(path, []byte(s+"\n"), fileMode); err != nil {
		return nil, fmt.Errorf("auth: write %s: %w", path, err)
	}
	log.Printf("auth: generated new token at %s (mode %#o)", path, fileMode)
	return &Token{value: s}, nil
}

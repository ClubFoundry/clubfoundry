package state

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// FinalizeSelfFromSentinels applies correlated results and sweeps stale files.
func FinalizeSelfFromSentinels(dataDir string, currentSidecarVersion string, selfState *State) (int, error) {
	dir := recreateStatusDir(dataDir)
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read sentinel dir %s: %w", dir, err)
	}

	processed := 0
	now := time.Now()
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		info, err := ent.Info()
		if err != nil {
			log.Printf("sentinel: stat %s: %v — skipping", path, err)
			continue
		}
		if now.Sub(info.ModTime()) > 24*time.Hour {
			if removeErr := os.Remove(path); removeErr != nil {
				log.Printf("sentinel: 24h sweep failed for %s: %v", path, removeErr)
			} else {
				log.Printf("sentinel: 24h sweep removed %s", path)
			}
			continue
		}

		sent, readErr := readSentinel(path)
		if readErr != nil {
			log.Printf("sentinel: read %s: %v — leaving for manual inspection", path, readErr)
			continue
		}
		currentOpID := selfState.OpID()
		if sent.OpID != "" && currentOpID != "" && sent.OpID != currentOpID {
			log.Printf("sentinel: discarding stale %s (op_id=%q vs current=%q)", path, sent.OpID, currentOpID)
			if removeErr := os.Remove(path); removeErr != nil {
				log.Printf("sentinel: remove stale %s: %v (will be swept after 24h)", path, removeErr)
			}
			continue
		}
		if (sent.OpID == "" || currentOpID == "") && !selfState.OpStartedAt().IsZero() {
			t, perr := time.Parse(time.RFC3339, sent.CompletedAt)
			if perr != nil {
				log.Printf("sentinel: discarding %s (CompletedAt=%q does not parse as RFC3339: %v)", path, sent.CompletedAt, perr)
				if removeErr := os.Remove(path); removeErr != nil {
					log.Printf("sentinel: remove malformed %s: %v", path, removeErr)
				}
				continue
			}
			if t.Before(selfState.OpStartedAt().Add(-60 * time.Second)) {
				log.Printf("sentinel: discarding time-stale %s (completed_at=%s, op started=%s)", path, sent.CompletedAt, selfState.OpStartedAt().Format(time.RFC3339))
				if removeErr := os.Remove(path); removeErr != nil {
					log.Printf("sentinel: remove stale %s: %v", path, removeErr)
				}
				continue
			}
		}
		applySentinelToSelfState(selfState, sent, currentSidecarVersion)
		if removeErr := os.Remove(path); removeErr != nil {
			log.Printf("sentinel: remove processed %s: %v (will be swept after 24h)", path, removeErr)
		}
		processed++
	}
	return processed, nil
}

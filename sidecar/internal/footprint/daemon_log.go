package footprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeJSONLLog appends one record per outcome to the daily prune log.
func writeJSONLLog(dir string, when time.Time, outcomes []PruneOutcome) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("auto-prune-%s.jsonl", when.Format("2006-01-02"))
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, o := range outcomes {
		b, err := json.Marshal(o)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func summarize(outs []PruneOutcome) (removed, kept, errors int) {
	for _, o := range outs {
		switch o.Action {
		case "removed":
			removed++
		case "error":
			errors++
		case "kept_hard", "kept_keepN", "kept_grace", "kept_in_use":
			kept++
		}
	}
	return
}

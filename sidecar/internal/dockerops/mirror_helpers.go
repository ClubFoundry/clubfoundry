package dockerops

import (
	"fmt"
	"time"
)

// forwardProgress serializes mirror progress through one callback.
func forwardProgress(fn ProgressCallback, stop <-chan struct{}, pick func() (int64, int64)) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last int64
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			b, total := pick()
			bps := float64(b-last) * 2
			if bps < 0 {
				bps = 0
			}
			last = b
			fn(b, total, bps)
		}
	}
}

// dedupeNonEmpty preserves first-seen mirror order.
func dedupeNonEmpty(urls []string) []string {
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// firstMirrorErr returns the first terminal source error.
func firstMirrorErr(mirrors []*mirrorDownload) error {
	for _, m := range mirrors {
		if m.failed.Load() {
			if e := <-m.done; e != nil {
				return e
			}
		}
	}
	return fmt.Errorf("no successful mirror")
}

// classifyDownloadErr preserves the underlying error for caller classification.
func classifyDownloadErr(err error) error {
	return err
}

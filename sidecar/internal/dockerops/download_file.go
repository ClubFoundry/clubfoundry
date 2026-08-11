package dockerops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
)

// downloadToFile writes one SHA-verified artifact and tracks live progress.
func (c Config) downloadToFile(ctx context.Context, m *mirrorDownload, wantSha string, progressFn ProgressCallback, logW io.Writer) error {
	if wantSha == "" {
		return fmt.Errorf("download requires sha256 (URL=%s); refusing to write unverified tarball", m.url)
	}
	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, m.url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	// The shared parent deadline and stall watchdog govern the transfer.
	resp, err := cloud.SharedChain().HTTPClient(0).Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", m.url, classifyDownloadErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: HTTP %d", m.url, resp.StatusCode)
	}
	bytesTotal := resp.ContentLength
	if bytesTotal > 0 {
		m.total.Store(bytesTotal)
	}

	f, err := os.Create(m.path)
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", m.path, err)
	}
	defer f.Close()

	hasher := sha256.New()
	pr := newProgressReader(resp.Body, bytesTotal, func(dl, total int64, bps float64) {
		m.bytes.Store(dl)
		if total > 0 {
			m.total.Store(total)
		}
		if progressFn != nil {
			progressFn(dl, total, bps)
		}
	})

	// Cancel a source that stops producing bytes without closing its body.
	var stalled atomic.Bool
	watchDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var lastBytes int64
		lastChange := time.Now()
		for {
			select {
			case <-watchDone:
				return
			case <-dlCtx.Done():
				return
			case <-ticker.C:
				b, _ := pr.Snapshot()
				if b > lastBytes {
					lastBytes, lastChange = b, time.Now()
					continue
				}
				if time.Since(lastChange) >= stallTimeout {
					stalled.Store(true)
					writeLog(logW, "stall watchdog: no data from %s for %s — aborting this source\n", m.url, stallTimeout)
					cancel()
					return
				}
			}
		}
	}()

	_, copyErr := io.Copy(f, io.TeeReader(pr, hasher))
	close(watchDone)
	if copyErr != nil {
		if stalled.Load() {
			return fmt.Errorf("fetch %s: %w", m.url, ErrSlowNetwork)
		}
		return fmt.Errorf("download %s: %w", m.url, classifyDownloadErr(copyErr))
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", m.path, err)
	}

	if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, wantSha) {
		return fmt.Errorf("sha256 mismatch on %s: got %s, want %s", m.url, got, wantSha)
	}
	if progressFn != nil && bytesTotal > 0 {
		progressFn(bytesTotal, bytesTotal, 0)
	}
	return nil
}

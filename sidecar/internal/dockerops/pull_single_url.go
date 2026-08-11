package dockerops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
)

// loadFromURL verifies one downloaded artifact before importing it.
func (c Config) loadFromURL(ctx context.Context, service, tag string, opts PullOpts) error {
	if opts.Sha256 == "" {
		return fmt.Errorf("URL-based pull requires sha256 (URL=%s); refusing to load unverified tarball", opts.URL)
	}

	if ref, err := c.PreloadedImageRef(service, tag); err == nil && c.HasImage(ctx, ref) {
		writeLog(opts.LogWriter,
			"image %s already present locally — skipping tarball download from %s (trust: local docker daemon by tag; sha256 verification of tarball is bypassed)\n",
			ref, opts.URL)
		if err := c.SetServiceImage(service, tag); err != nil {
			return fmt.Errorf("set image to %s after skip-download: %w", tag, err)
		}
		return nil
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultDownloadTimeout
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	writeLog(opts.LogWriter, "GET %s (timeout %s, expected sha256 %s)\n", opts.URL, timeout, opts.Sha256)

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	client := cloud.SharedChain().HTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", opts.URL, classifyDownloadErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: HTTP %d", opts.URL, resp.StatusCode)
	}

	bytesTotal := resp.ContentLength
	writeLog(opts.LogWriter, "response OK, Content-Length=%d\n", bytesTotal)

	dir, err := os.MkdirTemp(filepath.Join(c.ComposeDir, "data"), ".cf-update-")
	if err != nil {
		dir, err = os.MkdirTemp("", ".cf-update-")
		if err != nil {
			return fmt.Errorf("create temp dir for URL download: %w", err)
		}
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "image.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", path, err)
	}
	fileClosed := false
	defer func() {
		if !fileClosed {
			_ = f.Close()
		}
	}()

	pr := newProgressReader(resp.Body, bytesTotal, opts.ProgressFn)
	hasher := sha256.New()

	watchCtx, watchCancel := context.WithCancel(dlCtx)
	defer watchCancel()
	var watchdogTripped atomic.Bool
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				bytesRead, elapsed := pr.Snapshot()
				if elapsed < slowNetworkDwell {
					continue
				}
				avgBps := float64(bytesRead) / elapsed.Seconds()
				if avgBps < float64(slowNetworkThresholdBps) {
					writeLog(opts.LogWriter,
						"slow_network watchdog: %d bytes in %s = %.0f B/s (< 5 KB/s) — aborting download\n",
						bytesRead, elapsed.Truncate(time.Second), avgBps)
					watchdogTripped.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	_, copyErr := io.Copy(f, io.TeeReader(pr, hasher))
	watchCancel()
	if copyErr != nil {
		if watchdogTripped.Load() {
			return fmt.Errorf("fetch %s: %w", opts.URL, ErrSlowNetwork)
		}
		return fmt.Errorf("download %s: %w", opts.URL, classifyDownloadErr(copyErr))
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	fileClosed = true

	if opts.ProgressFn != nil && bytesTotal > 0 {
		opts.ProgressFn(bytesTotal, bytesTotal, 0)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, opts.Sha256) {
		return fmt.Errorf("sha256 mismatch on %s: got %s, want %s", opts.URL, got, opts.Sha256)
	}

	// The legacy single-URL path did not invoke this mirror-specific callback.
	loadOpts := opts
	loadOpts.OnLoadStart = nil
	return c.loadImageFromFile(dlCtx, service, tag, path, loadOpts)
}

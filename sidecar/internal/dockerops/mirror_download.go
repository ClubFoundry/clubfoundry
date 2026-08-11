package dockerops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// mirrorDownload tracks one isolated source in the mirror race.
type mirrorDownload struct {
	url    string
	path   string
	bytes  atomic.Int64
	total  atomic.Int64
	failed atomic.Bool
	cancel context.CancelFunc
	done   chan error
}

// loadFromURLs races mirrors by transferred bytes, then loads one verified file.
// Late winner failures fall back sequentially to the remaining sources.
func (c Config) loadFromURLs(ctx context.Context, service, tag string, opts PullOpts) error {
	// A matching local tag avoids all mirror traffic.
	if ref, err := c.PreloadedImageRef(service, tag); err == nil && c.HasImage(ctx, ref) {
		writeLog(opts.LogWriter,
			"image %s already present locally — skipping multi-mirror download of %d URL(s) (trust: local docker daemon by tag)\n",
			ref, len(opts.URLs))
		if err := c.SetServiceImage(service, tag); err != nil {
			return fmt.Errorf("set image to %s after skip-download: %w", tag, err)
		}
		return nil
	}

	urls := dedupeNonEmpty(opts.URLs)
	if len(urls) == 0 {
		return fmt.Errorf("loadFromURLs: empty URL list")
	}
	// One source does not need temporary race files.
	if len(urls) == 1 {
		attempt := opts
		attempt.URL = urls[0]
		attempt.URLs = nil
		return c.loadFromURL(ctx, service, tag, attempt)
	}
	if opts.Sha256 == "" {
		return fmt.Errorf("multi-mirror pull requires sha256; refusing to load unverified tarball")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultDownloadTimeout
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// One temporary directory owns every winner and loser artifact.
	dir, err := os.MkdirTemp(filepath.Join(c.ComposeDir, "data"), ".cf-update-")
	if err != nil {
		dir, err = os.MkdirTemp("", ".cf-update-")
		if err != nil {
			return fmt.Errorf("create temp dir for multi-mirror download: %w", err)
		}
	}
	defer os.RemoveAll(dir)

	writeLog(opts.LogWriter, "multi-mirror: racing %d sources, keep-fastest after %s (sha256 %s)\n",
		len(urls), raceWindow, opts.Sha256)

	// Each mirror writes independently and reports through a buffered channel.
	mirrors := make([]*mirrorDownload, len(urls))
	doneCount := make(chan int, len(urls))
	for i, u := range urls {
		mctx, mcancel := context.WithCancel(dlCtx)
		m := &mirrorDownload{
			url:    u,
			path:   filepath.Join(dir, fmt.Sprintf("mirror-%d.tar.gz", i)),
			cancel: mcancel,
			done:   make(chan error, 1),
		}
		mirrors[i] = m
		go func(idx int, m *mirrorDownload, mctx context.Context) {
			// A single leader forwarder owns the shared progress callback.
			err := c.downloadToFile(mctx, m, opts.Sha256, nil, opts.LogWriter)
			if err != nil {
				m.failed.Store(true)
			}
			m.done <- err
			doneCount <- idx
		}(i, m, mctx)
	}

	// Forward the current healthy leader so the progress bar does not restart.
	stopRace := make(chan struct{})
	var stopRaceOnce sync.Once
	stopReporting := func() { stopRaceOnce.Do(func() { close(stopRace) }) }
	defer stopReporting()
	if opts.ProgressFn != nil {
		go forwardProgress(opts.ProgressFn, stopRace, func() (int64, int64) {
			var b, total int64
			for _, m := range mirrors {
				if m.failed.Load() {
					continue
				}
				if got := m.bytes.Load(); got > b {
					b, total = got, m.total.Load()
				}
			}
			return b, total
		})
	}

	// Stop racing on timeout, completion of all sources, or cancellation.
	finished := 0
	timer := time.NewTimer(raceWindow)
	defer timer.Stop()
raceLoop:
	for finished < len(mirrors) {
		select {
		case <-dlCtx.Done():
			break raceLoop
		case <-timer.C:
			break raceLoop
		case <-doneCount:
			finished++
		}
	}

	// The healthy source with most bytes wins.
	winner, best := -1, int64(-1)
	for i, m := range mirrors {
		if m.failed.Load() {
			continue
		}
		if b := m.bytes.Load(); b > best {
			best, winner = b, i
		}
	}
	if winner == -1 {
		for _, m := range mirrors {
			m.cancel()
		}
		return fmt.Errorf("all %d mirror(s) failed during initial race: %w", len(mirrors), firstMirrorErr(mirrors))
	}

	// Partial loser files remain owned by the deferred directory cleanup.
	for i, m := range mirrors {
		if i != winner {
			m.cancel()
		}
	}
	win := mirrors[winner]
	writeLog(opts.LogWriter, "multi-mirror: winner = [%d] %s (%d KB pulled in race window)\n",
		winner, win.url, win.bytes.Load()/1024)

	// Stop byte progress before docker load or fallback starts.
	winErr := <-win.done
	stopReporting()

	if winErr == nil {
		if loadErr := c.loadImageFromFile(dlCtx, service, tag, win.path, opts); loadErr == nil {
			return nil
		} else {
			writeLog(opts.LogWriter, "multi-mirror: winner downloaded OK but docker load failed: %v — falling back\n", loadErr)
			winErr = loadErr
		}
	} else {
		writeLog(opts.LogWriter, "multi-mirror: winner %s failed: %v — falling back to other sources\n", win.url, winErr)
	}

	// Retry other sources from clean files after any late winner failure.
	lastErr := winErr
	for i, m := range mirrors {
		if i == winner {
			continue
		}
		if ctx.Err() != nil {
			return fmt.Errorf("multi-mirror cancelled during fallback: %w", ctx.Err())
		}
		writeLog(opts.LogWriter, "[fallback] %s\n", m.url)
		fb := &mirrorDownload{
			url:  m.url,
			path: filepath.Join(dir, fmt.Sprintf("fallback-%d.tar.gz", i)),
		}
		if err := c.downloadToFile(dlCtx, fb, opts.Sha256, opts.ProgressFn, opts.LogWriter); err != nil {
			writeLog(opts.LogWriter, "[fallback] %s download failed: %v\n", m.url, err)
			lastErr = err
			continue
		}
		if err := c.loadImageFromFile(dlCtx, service, tag, fb.path, opts); err != nil {
			writeLog(opts.LogWriter, "[fallback] %s docker load failed: %v\n", m.url, err)
			lastErr = err
			continue
		}
		writeLog(opts.LogWriter, "[fallback] %s success — done\n", m.url)
		return nil
	}
	return fmt.Errorf("all %d mirror(s) failed: %w", len(mirrors), lastErr)
}

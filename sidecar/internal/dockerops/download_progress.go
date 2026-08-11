package dockerops

import (
	"io"
	"sync"
	"time"
)

// progressReader samples an io.Reader safely across watchdog and status readers.
type progressReader struct {
	mu              sync.Mutex
	r               io.Reader
	total           int64
	startedAt       time.Time
	bytesRead       int64
	lastSampleAt    time.Time
	lastSampleBytes int64
	cb              ProgressCallback
}

// Snapshot returns synchronized bytes and elapsed time for watchdog checks.
func (p *progressReader) Snapshot() (bytesRead int64, elapsed time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bytesRead, time.Since(p.startedAt)
}

func newProgressReader(r io.Reader, total int64, cb ProgressCallback) *progressReader {
	now := time.Now()
	return &progressReader{
		r:            r,
		total:        total,
		startedAt:    now,
		lastSampleAt: now,
		cb:           cb,
	}
}

// Read tracks download progress and enforces the slow-network limit.
func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	now := time.Now()

	p.mu.Lock()
	p.bytesRead += int64(n)
	bytesRead := p.bytesRead
	startedAt := p.startedAt
	lastSampleAt := p.lastSampleAt
	lastSampleBytes := p.lastSampleBytes
	// Invoke callbacks after unlocking so observers cannot block reads.
	cb := p.cb
	emitSample := false
	var sampleBps float64
	if cb != nil && (now.Sub(lastSampleAt) >= 500*time.Millisecond || err != nil) {
		windowSecs := now.Sub(lastSampleAt).Seconds()
		if windowSecs > 0 {
			sampleBps = float64(bytesRead-lastSampleBytes) / windowSecs
		}
		p.lastSampleAt = now
		p.lastSampleBytes = bytesRead
		emitSample = true
	}
	p.mu.Unlock()

	// Active but persistently slow reads fail independently of callbacks.
	elapsed := now.Sub(startedAt)
	if elapsed >= slowNetworkDwell && err == nil {
		avgBps := float64(bytesRead) / elapsed.Seconds()
		if avgBps < float64(slowNetworkThresholdBps) {
			return n, ErrSlowNetwork
		}
	}

	if emitSample {
		cb(bytesRead, p.total, sampleBps)
	}
	return n, err
}

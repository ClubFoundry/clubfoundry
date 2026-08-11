package dockerops

import (
	"errors"
	"io"
	"testing"
	"time"
)

func TestProgressReader_SnapshotConcurrentSafe(t *testing.T) {
	// Drain a tiny payload while a second goroutine repeatedly calls
	// Snapshot. Race detector picks up any unsynchronized access.
	src := io.NopCloser(&slowBytes{remaining: 200, perRead: 10})
	pr := newProgressReader(src, 200, nil)

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = pr.Snapshot()
			}
		}
	}()

	buf := make([]byte, 32)
	total := 0
	for {
		n, err := pr.Read(buf)
		total += n
		if err != nil {
			break
		}
	}
	close(stop)

	if total != 200 {
		t.Errorf("expected 200 bytes drained, got %d", total)
	}
	bytes, elapsed := pr.Snapshot()
	if bytes != 200 {
		t.Errorf("Snapshot bytes = %d, want 200", bytes)
	}
	// elapsed is non-negative (a real bug would be a negative Duration from
	// a clock skip). On fast Windows hosts the monotonic clock can return
	// 0 within a single tick; we assert non-negative + Snapshot doesn't
	// crash, not "elapsed > 0".
	if elapsed < 0 {
		t.Errorf("Snapshot elapsed = %v, want >= 0", elapsed)
	}
}

func TestProgressReader_BailFiresInRead_WhenSlow(t *testing.T) {
	// 5 bytes per Read at no-delay = throughput >> threshold normally,
	// but we artificially backdate startedAt so elapsed > dwell on the
	// first Read iteration. avgBps then = bytesRead/elapsed which we
	// can force below 50KB/s by reading very few bytes.
	src := &slowBytes{remaining: 100, perRead: 1}
	pr := newProgressReader(io.NopCloser(src), 100, nil)
	// Force elapsed past dwell + ensure avgBps stays low: 1 byte / 61s = 0 KB/s.
	pr.startedAt = time.Now().Add(-2 * slowNetworkDwell)

	buf := make([]byte, 32)
	_, err := pr.Read(buf)
	if !errors.Is(err, ErrSlowNetwork) {
		t.Errorf("expected ErrSlowNetwork after dwell with low throughput, got %v", err)
	}
}

func TestProgressReader_BailDoesNotFireWhenFast(t *testing.T) {
	// 100 bytes returned in one fast Read, well above threshold.
	src := &slowBytes{remaining: 100, perRead: 100}
	pr := newProgressReader(io.NopCloser(src), 100, nil)
	pr.startedAt = time.Now().Add(-2 * slowNetworkDwell)
	// 100 bytes / 120s = ~0.8 B/s — would normally bail. So scale up:
	// pretend we already read a lot earlier.
	pr.bytesRead = slowNetworkThresholdBps * 1000 // way above threshold

	buf := make([]byte, 32)
	_, err := pr.Read(buf)
	if errors.Is(err, ErrSlowNetwork) {
		t.Error("expected no slow-network bail when avgBps high enough")
	}
}

// slowBytes implements io.Reader returning at most perRead bytes per
// call until remaining is exhausted. Used to simulate a constrained
// network without sleeping. Pointer receiver so mutation persists
// across consecutive Read calls.
type slowBytes struct {
	remaining int
	perRead   int
}

func (s *slowBytes) Read(p []byte) (int, error) {
	if s.remaining == 0 {
		return 0, io.EOF
	}
	n := s.perRead
	if n > len(p) {
		n = len(p)
	}
	if n > s.remaining {
		n = s.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = byte(i % 256)
	}
	s.remaining -= n
	return n, nil
}

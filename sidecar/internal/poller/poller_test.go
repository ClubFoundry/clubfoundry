package poller

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInsideWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	cases := []struct {
		now time.Time
		win string
		in  bool
	}{
		{at(3, 30), "03:00-05:00", true},
		{at(5, 0), "03:00-05:00", false}, // end is exclusive
		{at(2, 59), "03:00-05:00", false},
		{at(23, 30), "22:00-03:00", true}, // wraps
		{at(2, 30), "22:00-03:00", true},  // wraps
		{at(4, 0), "22:00-03:00", false},  // outside wrap
		{at(12, 0), "bad-window", false},  // malformed
	}
	for _, c := range cases {
		if got := insideWindow(c.now, c.win); got != c.in {
			t.Errorf("insideWindow(%v, %q) = %v, want %v", c.now.Format("15:04"), c.win, got, c.in)
		}
	}
}

func TestSidecarIsFrozen(t *testing.T) {
	t.Setenv("CLM_SIDECAR_FROZEN_VERSION", "  v3.TEST  ")
	frozen, version := sidecarIsFrozen()
	if !frozen || version != "v3.TEST" {
		t.Fatalf("sidecarIsFrozen() = (%v, %q)", frozen, version)
	}

	t.Setenv("CLM_SIDECAR_FROZEN_VERSION", "   ")
	frozen, version = sidecarIsFrozen()
	if frozen || version != "" {
		t.Fatalf("sidecarIsFrozen() for blank value = (%v, %q)", frozen, version)
	}
}

func TestCatalogManaged(t *testing.T) {
	for _, value := range []string{"truenas_apps", " TRUENAS_APPS "} {
		t.Setenv("CLM_UPDATE_MODE", value)
		if !catalogManaged() {
			t.Fatalf("catalogManaged() = false for %q", value)
		}
	}

	for _, value := range []string{"", "standalone", "truenas-apps"} {
		t.Setenv("CLM_UPDATE_MODE", value)
		if catalogManaged() {
			t.Fatalf("catalogManaged() = true for %q", value)
		}
	}
}

func TestParseHHMM(t *testing.T) {
	for input, want := range map[string]int{
		"00:00": 0,
		"23:59": 23*60 + 59,
		"24:00": -1,
		"12:60": -1,
		"9:00":  -1,
		"bad":   -1,
	} {
		if got := parseHHMM(input); got != want {
			t.Fatalf("parseHHMM(%q) = %d, want %d", input, got, want)
		}
	}
}

// TestFrozenMode_ShortCircuit verifies CLM_SIDECAR_FROZEN_VERSION makes
// `tick` return BEFORE any cloud or docker work. The frozen-check sits ahead
// of the nil-guard, so we can pass an empty Deps{} and still get an immediate
// return without a panic. Adversarial / verification runs on test boxes rely
// on this — the poller must never reach cloud.CheckUpdates while frozen.
func TestFrozenMode_ShortCircuit(t *testing.T) {
	t.Setenv("CLM_SIDECAR_FROZEN_VERSION", "v1.TEST-FROZEN")
	// Reset the sync.Once so the log line emits within this test even if
	// another test (in this package, this run) already triggered it.
	frozenLogOnce = sync.Once{}

	var buf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOut)

	deps := &Deps{} // intentionally empty — nil deps would normally panic later in tick
	deps.tick(context.Background())

	if !strings.Contains(buf.String(), "CLM_SIDECAR_FROZEN_VERSION") ||
		!strings.Contains(buf.String(), "v1.TEST-FROZEN") ||
		!strings.Contains(buf.String(), "auto-update loop disabled") {
		t.Errorf("expected frozen-mode log line; got: %q", buf.String())
	}
}

// TestFrozenMode_LogsOnceOnly verifies the log line is emitted once per process
// boot even if `tick` fires repeatedly — the poller runs hourly by default,
// 24 lines/day of the same message would be log noise.
func TestFrozenMode_LogsOnceOnly(t *testing.T) {
	t.Setenv("CLM_SIDECAR_FROZEN_VERSION", "v1.TEST-FROZEN")
	frozenLogOnce = sync.Once{}

	var buf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOut)

	deps := &Deps{}
	deps.tick(context.Background())
	deps.tick(context.Background())
	deps.tick(context.Background())

	count := strings.Count(buf.String(), "auto-update loop disabled")
	if count != 1 {
		t.Errorf("expected exactly 1 frozen-mode log line across 3 ticks, got %d. Buf: %q", count, buf.String())
	}
}

// TestFrozenMode_OffByDefault verifies normal poller behavior when the env
// var is unset — the frozen-check returns false and execution proceeds to
// the nil-guard (which catches our empty Deps and returns without panic).
// This is the critical backward-compatibility guarantee: production sidecars
// without the env var see ZERO behavior change.
func TestFrozenMode_OffByDefault(t *testing.T) {
	// Explicitly unset (in case parent shell had it set)
	t.Setenv("CLM_SIDECAR_FROZEN_VERSION", "")
	frozenLogOnce = sync.Once{}

	var buf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOut)

	deps := &Deps{}
	deps.tick(context.Background())

	if strings.Contains(buf.String(), "auto-update loop disabled") {
		t.Errorf("frozen-mode log line emitted when env var was unset; got: %q", buf.String())
	}
}

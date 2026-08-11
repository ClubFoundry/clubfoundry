package monitor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// fakeReinstall implements ReinstallTrigger, counts calls, and returns a
// configured error.
type fakeReinstall struct {
	calls atomic.Int32
	err   error
}

func (f *fakeReinstall) RunReinstallCurrent(ctx context.Context) error {
	f.calls.Add(1)
	return f.err
}

type fakeRollback struct {
	calls atomic.Int32
	last  time.Time
	err   error
}

func (f *fakeRollback) RunRollback(context.Context) error {
	f.calls.Add(1)
	return f.err
}

func (f *fakeRollback) LastSuccessfulMainUpdate() time.Time {
	return f.last
}

func TestNewDefaults(t *testing.T) {
	m := New(nil, dockerops.Config{}, state.New(), nil)
	if m.ProbeInterval != 60*time.Second {
		t.Fatalf("ProbeInterval = %s, want 60s", m.ProbeInterval)
	}
	if m.FailThreshold != 3 {
		t.Fatalf("FailThreshold = %d, want 3", m.FailThreshold)
	}
	if m.RestartWindow != time.Hour || m.MaxRestartsInWin != 5 {
		t.Fatalf("restart policy = %s/%d, want 1h/5", m.RestartWindow, m.MaxRestartsInWin)
	}
	if m.PostUpdateRollbackWindow != time.Hour {
		t.Fatalf("PostUpdateRollbackWindow = %s, want 1h", m.PostUpdateRollbackWindow)
	}
	if m.AutoSoftRecoverWindow != state.AutoSoftRecoverMinDuration {
		t.Fatalf("AutoSoftRecoverWindow = %s, want %s", m.AutoSoftRecoverWindow, state.AutoSoftRecoverMinDuration)
	}
}

func TestTryAutoRollbackEligibility(t *testing.T) {
	tests := []struct {
		name      string
		trigger   *fakeRollback
		want      bool
		wantCalls int32
	}{
		{name: "no trigger"},
		{name: "no successful update", trigger: &fakeRollback{}},
		{
			name:    "outside rollback window",
			trigger: &fakeRollback{last: time.Now().Add(-2 * time.Hour)},
		},
		{
			name:      "recent update",
			trigger:   &fakeRollback{last: time.Now().Add(-time.Minute)},
			want:      true,
			wantCalls: 1,
		},
		{
			name:      "rollback failure",
			trigger:   &fakeRollback{last: time.Now().Add(-time.Minute), err: errors.New("rollback failed")},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var trigger RollbackTrigger
			if tt.trigger != nil {
				trigger = tt.trigger
			}
			m := &Monitor{
				Rollback:                 trigger,
				PostUpdateRollbackWindow: time.Hour,
			}
			if got := m.tryAutoRollback(context.Background()); got != tt.want {
				t.Fatalf("tryAutoRollback() = %v, want %v", got, tt.want)
			}
			if tt.trigger != nil && tt.trigger.calls.Load() != tt.wantCalls {
				t.Fatalf("RunRollback calls = %d, want %d", tt.trigger.calls.Load(), tt.wantCalls)
			}
		})
	}
}

func TestPruneDropsOldTimestamps(t *testing.T) {
	m := &Monitor{RestartWindow: time.Hour}
	now := time.Now()
	m.upAttempts = []time.Time{
		now.Add(-2 * time.Hour),    // older than window
		now.Add(-90 * time.Minute), // older than window
		now.Add(-30 * time.Minute), // inside window
		now.Add(-5 * time.Minute),  // inside window
	}
	m.prune(now)
	if len(m.upAttempts) != 2 {
		t.Fatalf("want 2 upAttempts after prune, got %d", len(m.upAttempts))
	}
}

func TestTooManyRecentUpAttempts(t *testing.T) {
	m := &Monitor{RestartWindow: time.Hour, MaxRestartsInWin: 3}
	now := time.Now()
	m.upAttempts = []time.Time{
		now.Add(-30 * time.Minute),
		now.Add(-20 * time.Minute),
	}
	if m.tooManyRecentUpAttempts() {
		t.Errorf("2 attempts should not exceed cap of 3")
	}
	m.upAttempts = append(m.upAttempts, now.Add(-5*time.Minute))
	if !m.tooManyRecentUpAttempts() {
		t.Errorf("3 attempts should hit cap of 3")
	}
}

// TestEscalationGap_FailedUpsCountTowardEscalation verifies that every Up
// attempt counts toward MaxRestartsInWin, including failed attempts.
func TestEscalationGap_FailedUpsCountTowardEscalation(t *testing.T) {
	m := &Monitor{RestartWindow: time.Hour, MaxRestartsInWin: 5}
	now := time.Now()
	// Simulate five failed Up attempts without a healthy probe.
	m.upAttempts = []time.Time{
		now.Add(-30 * time.Minute),
		now.Add(-25 * time.Minute),
		now.Add(-20 * time.Minute),
		now.Add(-15 * time.Minute),
		now.Add(-10 * time.Minute),
	}
	if !m.tooManyRecentUpAttempts() {
		t.Fatalf("5 failed up-attempts must trip the escalation gate (was unreachable before fix)")
	}
}

// Same-version reinstall.

func TestTryAutoReinstall_NilTriggerReturnsFalse(t *testing.T) {
	m := &Monitor{}
	if m.tryAutoReinstall(context.Background()) {
		t.Fatalf("nil ReinstallTrigger should return false (legacy halt path)")
	}
}

func TestTryAutoReinstall_SuccessReturnsTrueAndRecords(t *testing.T) {
	fr := &fakeReinstall{err: nil}
	m := &Monitor{Reinstall: fr}
	if !m.tryAutoReinstall(context.Background()) {
		t.Fatalf("successful reinstall should return true")
	}
	if fr.calls.Load() != 1 {
		t.Fatalf("RunReinstallCurrent called %d times, want 1", fr.calls.Load())
	}
	if m.reinstallTriedAt.IsZero() {
		t.Fatalf("reinstallTriedAt should be set after successful attempt")
	}
}

func TestTryAutoReinstall_FailureReturnsFalseButStillRecords(t *testing.T) {
	fr := &fakeReinstall{err: errors.New("cloud timeout")}
	m := &Monitor{Reinstall: fr}
	if m.tryAutoReinstall(context.Background()) {
		t.Fatalf("failed reinstall should return false (caller halts)")
	}
	if fr.calls.Load() != 1 {
		t.Fatalf("RunReinstallCurrent called %d times, want 1", fr.calls.Load())
	}
	// Critical: even after failure we must record the attempt so a stuck
	// monitor doesn't tight-loop reinstall on every probe.
	if m.reinstallTriedAt.IsZero() {
		t.Fatalf("reinstallTriedAt MUST be set even after failure")
	}
}

func TestTryAutoReinstall_AlreadyTriedSkips(t *testing.T) {
	fr := &fakeReinstall{err: nil}
	m := &Monitor{Reinstall: fr, reinstallTriedAt: time.Now().Add(-5 * time.Minute)}
	if m.tryAutoReinstall(context.Background()) {
		t.Fatalf("already-tried path should return false (don't loop)")
	}
	if fr.calls.Load() != 0 {
		t.Fatalf("RunReinstallCurrent should not be called when already tried, got %d", fr.calls.Load())
	}
}

// TestAutoSoftRecover_MainState verifies that main state stuck in Error
// clears only after a sustained healthy probe window.
func TestAutoSoftRecover_MainState(t *testing.T) {
	st := state.New()
	if err := st.TransitionTo(state.Updating, ""); err != nil {
		t.Fatalf("transition: %v", err)
	}
	st.MarkError("SIM", "old failure")
	// 50ms past the 1ms window — same Windows-clock guard as state tests.
	time.Sleep(50 * time.Millisecond)
	m := &Monitor{
		State:                 st,
		AutoSoftRecoverWindow: 1 * time.Millisecond,
		ProbeInterval:         60 * time.Second, // probe pace doesn't matter — direct call
	}
	// consecOks=1 with window=1ms and interval=60s → required=1 (ceiling).
	m.maybeAutoSoftRecover(context.Background(), 1)
	if snap := st.Snapshot(); snap.Phase != state.Idle {
		t.Errorf("mainState not recovered: phase=%s lastErr=%q", snap.Phase, snap.LastError)
	}
}

// TestAutoSoftRecover_RequiresOks — fewer consecutive oks than required
// MUST not recover, even if state has been in Error long enough.
func TestAutoSoftRecover_RequiresOks(t *testing.T) {
	st := state.New()
	_ = st.TransitionTo(state.Updating, "")
	st.MarkError("SIM", "x")
	time.Sleep(50 * time.Millisecond)
	m := &Monitor{
		State:                 st,
		AutoSoftRecoverWindow: 10 * time.Minute,
		ProbeInterval:         60 * time.Second, // required = 10
	}
	m.maybeAutoSoftRecover(context.Background(), 5) // half the window
	if snap := st.Snapshot(); snap.Phase != state.Error {
		t.Errorf("mainState recovered prematurely with consecOks=5: %+v", snap)
	}
}

// TestAutoSoftRecover_SelfStateIndependent — selfState recovers
// independently of consecOks count (sidecar process being responsive
// is enough — no need to wait for main /health).
func TestAutoSoftRecover_SelfStateIndependent(t *testing.T) {
	self := state.New()
	_ = self.TransitionTo(state.Updating, "")
	self.MarkError("SELF_FAIL", "trampoline failed")
	time.Sleep(50 * time.Millisecond)
	m := &Monitor{
		SelfState:             self,
		AutoSoftRecoverWindow: 1 * time.Millisecond,
		ProbeInterval:         60 * time.Second,
	}
	// consecOks=0 — selfState path doesn't read it.
	m.maybeAutoSoftRecover(context.Background(), 0)
	if snap := self.Snapshot(); snap.Phase != state.Idle {
		t.Errorf("selfState not recovered independently: %+v", snap)
	}
}

// TestAutoSoftRecover_NoState — nil-safe when neither state wired.
func TestAutoSoftRecover_NoState(t *testing.T) {
	m := &Monitor{AutoSoftRecoverWindow: time.Millisecond}
	// Must not panic.
	m.maybeAutoSoftRecover(context.Background(), 100)
}

// TestAutoSoftRecover_ZeroWindowDisabled — when window is zero, the
// auto-recover is disabled (no-op).
func TestAutoSoftRecover_ZeroWindowDisabled(t *testing.T) {
	st := state.New()
	_ = st.TransitionTo(state.Updating, "")
	st.MarkError("SIM", "x")
	m := &Monitor{
		State:                 st,
		AutoSoftRecoverWindow: 0, // disabled
		ProbeInterval:         60 * time.Second,
	}
	m.maybeAutoSoftRecover(context.Background(), 1000)
	if snap := st.Snapshot(); snap.Phase != state.Error {
		t.Errorf("disabled auto-recover still fired: %+v", snap)
	}
}

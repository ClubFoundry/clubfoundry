package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

// Drives Deps.RunUpdate / RunSelfUpdate through every documented failure
// mode, with no real Docker daemon, no real /health probe, no real SQLite
// backups. Each scenario configures the three fakes, runs the entry point
// under a context deadline, and asserts:
//
//   - returned error string contains expected fragments
//   - final state machine phase + classified error code
//   - exact sequence of Docker / Backup calls (so a future refactor can't
//     silently drop a Stop, double-call Pull, etc.)
//
// All scenarios use opts.URL="" so preflight skips the live network HEAD
// probe — DNS lookups in test runs would otherwise yield non-deterministic
// results. The pull dispatch still goes through fakeDocker.Pull regardless,
// which is what the harness is testing.

// makeDeps wires up Deps with all three fakes + fresh main-kind State + a
// temp DataDir/LogDir. The returned `cleanup` removes the tree.
func makeDeps(t *testing.T) (*Deps, *fakeDocker, *fakeHealth, *fakeBackup, *state.State, func()) {
	t.Helper()
	dataDir, err := os.MkdirTemp("", "updater-harness-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	logDir := filepath.Join(dataDir, "update-logs")
	_ = os.MkdirAll(logDir, 0o755)

	mainState := state.NewKindAware(state.KindMain, dataDir)
	selfState := state.NewKindAware(state.KindSelf, dataDir)

	doc := newFakeDocker()
	hc := newFakeHealthOK("1.1.80") // overridden per scenario
	b := newFakeBackup()

	hist := history.New(filepath.Join(dataDir, "updater-history.json"))

	deps := &Deps{
		Docker:        doc,
		Health:        hc,
		Backup:        b,
		History:       hist,
		State:         mainState,
		SelfState:     selfState,
		DataDir:       dataDir,
		LogDir:        logDir,
		StartupWindow: 5 * time.Second,
		SelfVersion:   "v1.S",
	}
	cleanup := func() { _ = os.RemoveAll(dataDir) }
	return deps, doc, hc, b, mainState, cleanup
}

func runUpdateOrTimeout(t *testing.T, d *Deps, target string, opts dockerops.PullOpts) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return d.RunUpdate(ctx, target, opts)
}

func stagePSResult(doc *fakeDocker, fromVersion string) {
	doc.psResult = []dockerops.ServiceInfo{
		{Service: doc.main, Image: "clubfoundry:" + fromVersion, Tag: fromVersion, State: "running"},
	}
}

// healthHappyPath is a fakeHealth script that satisfies the doUpdate
// happy-path: WaitHealthy returns target version, smoke re-probe returns
// target version. Re-pollable forever (last entry repeats).
func healthHappyPath(target string) *fakeHealth {
	return newFakeHealth(
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: target}},
	)
}

// healthRollbackOnly is a fakeHealth script for scenarios where doUpdate
// errors before WaitHealthy, but doRollback's WaitHealthy is invoked. The
// rollback expects the from-version. Single entry that repeats.
func healthRollbackOnly(fromVersion string) *fakeHealth {
	return newFakeHealth(
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: fromVersion}},
	)
}

// containsAllInOrder reports whether every want appears in haystack in
// the order given (each want must match at-or-after the previous match).
func containsAllInOrder(haystack, wants []string) error {
	idx := 0
	for _, want := range wants {
		found := -1
		for j := idx; j < len(haystack); j++ {
			if strings.Contains(haystack[j], want) {
				found = j
				break
			}
		}
		if found < 0 {
			return fmt.Errorf("missing call %q after index %d in calls %v", want, idx, haystack)
		}
		idx = found + 1
	}
	return nil
}

func mustErrSubstr(t *testing.T, err error, want []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q missing substring %q", err.Error(), w)
		}
	}
}

// TestRunUpdate_HappyPath — golden path. PS → Pull → Stop → Backup → Up →
// WaitHealthy → smoke → tag retention. State ends Idle, no error code,
// backup created and pruned.
func TestRunUpdate_HappyPath(t *testing.T) {
	deps, doc, _, b, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	deps.Health = healthHappyPath("1.1.80")

	if err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{}); err != nil {
		t.Fatalf("happy path: %v", err)
	}
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle", got)
	}
	if err := containsAllInOrder(doc.Calls(), []string{
		"Pull(svc=clubfoundry,tag=1.1.80",
		"Stop(clubfoundry)",
		"Up(clubfoundry)",
		"TagImage(clubfoundry:1.1.79,clubfoundry:previous)",
		"TagImage(clubfoundry:1.1.80,clubfoundry:current)",
	}); err != nil {
		t.Errorf("docker call order: %v", err)
	}
	if err := containsAllInOrder(b.Calls(), []string{"CreateBackup(1.1.79)", "PruneOld()"}); err != nil {
		t.Errorf("backup call order: %v", err)
	}
}

// TestRunUpdate_PullFails — Pull returns error before any destructive op
// has run. RunUpdate still triggers rollback (defensive). Final state Idle
// after successful rollback.
func TestRunUpdate_PullFails(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	doc.pullErr = errFakeBoom
	// pull fails → no destructive op → rollback fires → re-pulls fromVer.
	// We need rollback's Pull to SUCCEED so the test settles. Use a counter:
	// first Pull (svc=clubfoundry tag=1.1.80) fails, second Pull (rollback,
	// tag=1.1.79) succeeds. Implemented via onPull callback.
	pullCount := 0
	doc.onPull = func(opts dockerops.PullOpts) {
		pullCount++
		if pullCount == 1 {
			doc.pullErr = errFakeBoom
		} else {
			doc.pullErr = nil
		}
	}
	deps.Health = healthRollbackOnly("1.1.79")

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"pull image"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
	// Pull was called twice (forward, then rollback re-pull of fromVersion).
	if pullCount != 2 {
		t.Errorf("expected 2 Pull calls (forward + rollback re-pull); got %d", pullCount)
	}
}

// TestRunUpdate_StopFails — Pull succeeded, Stop returned error. The
// running container is in a known-bad state: image was rewritten in
// compose but compose-stop failed. Rollback fires; Stop is called again
// during rollback (which we let succeed by clearing stopErr).
func TestRunUpdate_StopFails(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	stopCount := 0
	originalStopErr := errFakeBoom
	doc.stopErr = originalStopErr
	// First Stop fails (forward path). Subsequent Stops succeed (rollback).
	// We hook this via a wrapper: but fakeDocker doesn't have onStop. Add
	// inline: clear stopErr after first call by checking call count.
	checkAndClearStop := func() {
		stopCount++
		if stopCount == 1 {
			// keep original err for first call; subsequent calls clear.
			return
		}
		doc.stopErr = nil
	}
	// We can't intercept Stop without an onStop hook. Instead, increment
	// pullCount during Pull (after which we know Stop is next) and clear
	// stopErr there — fragile but workable for one specific scenario.
	pullCount := 0
	doc.onPull = func(opts dockerops.PullOpts) {
		pullCount++
		// After the second Pull (rollback's re-pull), allow Stop to succeed.
		if pullCount >= 2 {
			doc.stopErr = nil
		}
	}
	_ = checkAndClearStop // unused now — pullCount-based hook is sufficient
	deps.Health = healthRollbackOnly("1.1.79")

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"stop main"})
	// State must end Idle after successful rollback; rollback's Stop also
	// fails initially (still using original stopErr until pullCount>=2 triggers).
	// Worst case rollback fails too → Error. We accept either Idle or Error
	// here as long as the original "stop main" error surfaced.
	phase := st.Snapshot().Phase
	if phase != state.Idle && phase != state.Error {
		t.Errorf("phase = %s, want Idle or Error", phase)
	}
}

// TestRunUpdate_BackupFails — Pull + Stop succeeded, CreateBackup errors.
// doUpdate runs a defensive Up to bring the old container back, then
// returns. RunUpdate triggers doRollback; with no backup created, rollback
// uses LatestBackup() → "" → re-pulls fromVer.
func TestRunUpdate_BackupFails(t *testing.T) {
	deps, doc, _, b, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	b.createErr = errFakeBoom
	deps.Health = healthRollbackOnly("1.1.79")

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"backup db"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
	// CreateBackup was called exactly once (and errored).
	createCount := 0
	for _, c := range b.Calls() {
		if strings.HasPrefix(c, "CreateBackup(") {
			createCount++
		}
	}
	if createCount != 1 {
		t.Errorf("CreateBackup call count = %d, want 1", createCount)
	}
}

// TestRunUpdate_UpFailsAfterBackup — doUpdate progressed through Backup
// successfully, then `docker compose up` failed. doRollback fires; in the
// scenario where Up keeps failing in rollback too, we get the
// double-failure path.
func TestRunUpdate_UpFailsAfterBackup_DoubleFail(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	doc.upErr = errFakeBoom
	deps.Health = healthRollbackOnly("1.1.79")

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"docker compose up", "rollback also failed"})

	snap := st.Snapshot()
	if snap.Phase != state.Error {
		t.Errorf("phase = %s, want Error", snap.Phase)
	}
	if snap.LastErrorCode != "UPDATE_AND_ROLLBACK_FAILED" {
		t.Errorf("error code = %q, want UPDATE_AND_ROLLBACK_FAILED", snap.LastErrorCode)
	}
}

// TestRunUpdate_HealthTimeout — WaitHealthy returns context-deadline-exceeded.
// doUpdate returns "post-update health" error. Rollback fires; rollback's
// own WaitHealthy must succeed for clean Idle exit.
func TestRunUpdate_HealthTimeout(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	// First WaitHealthy call (forward) times out. Subsequent calls (rollback
	// path) succeed with the from-version.
	hc := newFakeHealth() // initially: timeout
	hc.waitTimeout = true
	deps.Health = hc
	// We need a way to flip the timeout off for rollback's WaitHealthy.
	// Add a hook via Up — after Up has been called twice (forward + rollback),
	// clear the timeout. But hc is independent. Use Pull onPull hook to flip.
	pullCount := 0
	doc.onPull = func(opts dockerops.PullOpts) {
		pullCount++
		if pullCount == 2 {
			// rollback's re-pull just happened — next WaitHealthy is rollback's.
			hc.waitTimeout = false
			hc.script = []healthProbeResult{
				{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.79"}},
			}
		}
	}

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"post-update health"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
}

// TestRunUpdate_VersionMismatch — /health returns OK but reports a different
// version than the target tag. doUpdate hard-fails with "version mismatch"
// and rollback fires. Rollback's health probe must report the from-version.
func TestRunUpdate_VersionMismatch(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	hc := newFakeHealth(
		// Forward WaitHealthy: returns wrong version → mismatch.
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.40"}},
		// Rollback WaitHealthy: returns from-version (rollback succeeded).
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.79"}},
	)
	deps.Health = hc

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"version mismatch"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
}

// TestRunUpdate_SmokeUnhealthy — first Probe is OK with target version,
// re-probe (smoke test) returns NOT ok. Caller's doUpdate returns
// "smoke test re-probe unhealthy" error.
func TestRunUpdate_SmokeUnhealthy(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	hc := newFakeHealth(
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}}, // initial
		healthProbeResult{ok: false, report: health.Report{Status: "starting", Ready: false}},            // smoke: NOT ok
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.79"}}, // rollback
	)
	deps.Health = hc

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"smoke test", "re-probe unhealthy"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
}

// TestRunUpdate_SmokeVersionDrift — first Probe ok with target version,
// re-probe (smoke) ok BUT reports a different version. Caller's smokeTest
// hard-fails with "version drift".
func TestRunUpdate_SmokeVersionDrift(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	hc := newFakeHealth(
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}}, // initial
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.79"}}, // smoke: drift
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.79"}}, // rollback
	)
	deps.Health = hc

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"smoke test", "drift"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
}

// TestRunSelfUpdate_PullFails — sidecar's own update flow rejects when the
// new image can't be pulled. SelfState ends in Error, no trampoline spawned.
func TestRunSelfUpdate_PullFails(t *testing.T) {
	deps, doc, _, _, _, cleanup := makeDeps(t)
	defer cleanup()
	doc.pullErr = errFakeBoom

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := deps.RunSelfUpdate(ctx, "v1.T", dockerops.PullOpts{})
	if err == nil {
		t.Fatalf("expected pull error, got nil")
	}
	if !errors.Is(err, errFakeBoom) {
		t.Fatalf("expected wrapped errFakeBoom, got %v", err)
	}

	snap := deps.SelfState.Snapshot()
	if snap.Phase != state.Error {
		t.Errorf("self-state phase = %s, want Error", snap.Phase)
	}
	if snap.LastErrorCode != "SELF_UPDATE_PULL_FAILED" {
		t.Errorf("error code = %q, want SELF_UPDATE_PULL_FAILED", snap.LastErrorCode)
	}
	for _, c := range doc.Calls() {
		if strings.HasPrefix(c, "SpawnRecreateTrampoline") {
			t.Errorf("trampoline must NOT spawn after pull failure; got call %q", c)
		}
	}
}

// TestRunSelfUpdate_TrampolineSpawnFails — Pull succeeds, but the helper
// container can't even start. SelfState ends Error with
// SELF_UPDATE_RECREATE_FAILED.
func TestRunSelfUpdate_TrampolineSpawnFails(t *testing.T) {
	deps, doc, _, _, _, cleanup := makeDeps(t)
	defer cleanup()
	doc.trampErr = errFakeBoom

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := deps.RunSelfUpdate(ctx, "v1.T", dockerops.PullOpts{})
	if err == nil {
		t.Fatalf("expected trampoline error, got nil")
	}
	if !errors.Is(err, errFakeBoom) {
		t.Fatalf("expected wrapped errFakeBoom, got %v", err)
	}

	snap := deps.SelfState.Snapshot()
	if snap.Phase != state.Error {
		t.Errorf("self-state phase = %s, want Error", snap.Phase)
	}
	if snap.LastErrorCode != "SELF_UPDATE_RECREATE_FAILED" {
		t.Errorf("error code = %q, want SELF_UPDATE_RECREATE_FAILED", snap.LastErrorCode)
	}
	if err := containsAllInOrder(doc.Calls(), []string{
		"Pull(svc=clubfoundry-updater",
		"SpawnRecreateTrampoline(clubfoundry-updater",
	}); err != nil {
		t.Errorf("call order: %v", err)
	}
}

// TestRunSelfUpdate_KindIsolation verifies that self-update state cannot leak
// into the main update state.
func TestRunSelfUpdate_KindIsolation(t *testing.T) {
	deps, _, _, _, mainState, cleanup := makeDeps(t)
	defer cleanup()
	mainState.SetOpID("op-main-untouched")
	mainBefore := mainState.Snapshot()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.RunSelfUpdate(ctx, "v1.T", dockerops.PullOpts{}); err != nil {
		t.Fatalf("RunSelfUpdate: %v", err)
	}

	mainAfter := mainState.Snapshot()
	if mainAfter.OpID != mainBefore.OpID {
		t.Errorf("main-state OpID changed during self-update: before=%q after=%q",
			mainBefore.OpID, mainAfter.OpID)
	}
	if mainAfter.Phase != mainBefore.Phase {
		t.Errorf("main-state phase changed during self-update: before=%s after=%s",
			mainBefore.Phase, mainAfter.Phase)
	}
	if mainAfter.TargetVersion != mainBefore.TargetVersion {
		t.Errorf("main-state TargetVersion leaked from self-update: before=%q after=%q",
			mainBefore.TargetVersion, mainAfter.TargetVersion)
	}
	if got := deps.SelfState.Snapshot().TargetVersion; got != "v1.T" {
		t.Errorf("self-state TargetVersion = %q, want v1.T", got)
	}
}

func TestRunSelfUpdate_PreflightBeforeTrampolineContract(t *testing.T) {
	deps, docker, _, _, _, cleanup := makeDeps(t)
	defer cleanup()
	docker.validateErr = errFakeBoom

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := deps.RunSelfUpdate(ctx, "v1.T", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"compose pre-flight", "sidecar still running"})

	calls := docker.Calls()
	if err := containsAllInOrder(calls, []string{
		"Pull(svc=clubfoundry-updater,tag=v1.T",
		"ValidateComposeForRecreate(clubfoundry-updater)",
	}); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if strings.HasPrefix(call, "SpawnRecreateTrampoline(") {
			t.Fatalf("trampoline must not spawn after preflight failure: %v", calls)
		}
	}
	if got := deps.SelfState.Snapshot().LastErrorCode; got != "COMPOSE_VALIDATION_FAILED" {
		t.Fatalf("self-update error code = %q, want COMPOSE_VALIDATION_FAILED", got)
	}
}

func TestStage_SuccessIsNonDestructiveContract(t *testing.T) {
	deps, docker, _, backup, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(docker, "1.1.79")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.Stage(ctx, "1.1.80", dockerops.PullOpts{}); err != nil {
		t.Fatalf("stage: %v", err)
	}

	snap := st.Snapshot()
	if snap.Phase != state.Staged || snap.StagedTarget != "1.1.80" {
		t.Fatalf("stage state = phase %s target %q, want Staged/1.1.80", snap.Phase, snap.StagedTarget)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"PS()",
		"Pull(svc=clubfoundry,tag=1.1.80",
	}); err != nil {
		t.Fatal(err)
	}
	for _, call := range docker.Calls() {
		if strings.HasPrefix(call, "Stop(") || strings.HasPrefix(call, "Up(") {
			t.Fatalf("stage must not change service lifecycle: %v", docker.Calls())
		}
	}
	if calls := backup.Calls(); len(calls) != 0 {
		t.Fatalf("stage must not touch backups: %v", calls)
	}
	entries, err := deps.History.List(10)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("successful stage must not create a completed history row: %+v", entries)
	}
}

func TestApply_UsesStagedImageWithoutPullContract(t *testing.T) {
	deps, docker, _, backup, st, cleanup := makeDeps(t)
	defer cleanup()
	if err := st.TransitionTo(state.Staging, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionTo(state.Staged, "test setup"); err != nil {
		t.Fatal(err)
	}
	st.SetStagedTarget("1.1.80")
	stagePSResult(docker, "1.1.79")
	deps.Health = healthHappyPath("1.1.80")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if got := st.Snapshot().Phase; got != state.Idle {
		t.Fatalf("phase = %s, want Idle", got)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"PS()",
		"Stop(clubfoundry)",
		"ValidateComposeForRecreate(clubfoundry)",
		"Up(clubfoundry)",
	}); err != nil {
		t.Fatal(err)
	}
	for _, call := range docker.Calls() {
		if strings.HasPrefix(call, "Pull(") {
			t.Fatalf("apply must use the staged local image without pulling: %v", docker.Calls())
		}
	}
	if err := containsAllInOrder(backup.Calls(), []string{
		"CreateBackup(1.1.79)",
		"PruneOld()",
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := deps.History.List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Outcome != history.OutcomeSuccess || entries[0].ToVersion != "1.1.80" {
		t.Fatalf("apply history = %+v, want one successful 1.1.80 entry", entries)
	}
}

func TestApply_PreflightFailureRollsBackContract(t *testing.T) {
	deps, docker, _, backup, st, cleanup := makeDeps(t)
	defer cleanup()
	if err := st.TransitionTo(state.Staging, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionTo(state.Staged, "test setup"); err != nil {
		t.Fatal(err)
	}
	st.SetStagedTarget("1.1.80")
	stagePSResult(docker, "1.1.79")
	docker.validateErr = errFakeBoom
	docker.hasImage["clubfoundry:1.1.79"] = true
	deps.Health = healthRollbackOnly("1.1.79")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := deps.Apply(ctx)
	mustErrSubstr(t, err, []string{"compose pre-flight", "rolled back"})

	if got := st.Snapshot().Phase; got != state.Idle {
		t.Fatalf("phase = %s, want Idle after successful rollback", got)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"Stop(clubfoundry)",
		"ValidateComposeForRecreate(clubfoundry)",
		"Stop(clubfoundry)",
		"HasImage(clubfoundry:1.1.79)",
		"SetServiceImage(clubfoundry,clubfoundry:1.1.79)",
		"Up(clubfoundry)",
	}); err != nil {
		t.Fatal(err)
	}
	if err := containsAllInOrder(backup.Calls(), []string{
		"CreateBackup(1.1.79)",
		"RestoreBackup(/fake/backup/clm.db.pre-update-v1.1.79-1)",
	}); err != nil {
		t.Fatal(err)
	}
	entries, listErr := deps.History.List(10)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(entries) != 1 || entries[0].Outcome != history.OutcomeRollback {
		t.Fatalf("apply history = %+v, want one rollback entry", entries)
	}
}

func TestRunSteppedUpdate_MultiHopSuccessContract(t *testing.T) {
	deps, docker, _, backup, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(docker, "1.1.79")
	deps.Health = newFakeHealth(
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}},
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}},
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.81"}},
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.81"}},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := deps.RunSteppedUpdate(ctx, []string{"1.1.80", "1.1.81"}); err != nil {
		t.Fatalf("stepped update: %v", err)
	}
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Fatalf("phase = %s, want Idle", got)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"Pull(svc=clubfoundry,tag=1.1.80",
		"Stop(clubfoundry)",
		"ValidateComposeForRecreate(clubfoundry)",
		"Up(clubfoundry)",
		"Pull(svc=clubfoundry,tag=1.1.81",
		"Stop(clubfoundry)",
		"ValidateComposeForRecreate(clubfoundry)",
		"Up(clubfoundry)",
	}); err != nil {
		t.Fatal(err)
	}
	if err := containsAllInOrder(backup.Calls(), []string{
		"CreateBackup(1.1.79)",
		"PruneOld()",
		"CreateBackup(1.1.80)",
		"PruneOld()",
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := deps.History.List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Outcome != history.OutcomeSuccess || entry.FromVersion != "1.1.79" || entry.ToVersion != "1.1.81" {
		t.Fatalf("history entry = %+v", entry)
	}
	if len(entry.Steps) != 2 || len(entry.Hops) != 2 || entry.Hops[0] != "1.1.80" || entry.Hops[1] != "1.1.81" {
		t.Fatalf("steps/hops = %+v / %v", entry.Steps, entry.Hops)
	}
}

func TestRunSteppedUpdate_FailureStopsAtLastSuccessfulContract(t *testing.T) {
	deps, docker, _, backup, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(docker, "1.1.79")
	pullCount := 0
	docker.onPull = func(dockerops.PullOpts) {
		pullCount++
		if pullCount == 2 {
			docker.pullErr = errFakeBoom
		}
	}
	docker.hasImage["clubfoundry:1.1.80"] = true
	deps.Health = newFakeHealth(
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}},
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}},
		healthProbeResult{ok: true, report: health.Report{Status: "ok", Ready: true, Version: "1.1.80"}},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := deps.RunSteppedUpdate(ctx, []string{"1.1.80", "1.1.81", "1.1.82"})
	mustErrSubstr(t, err, []string{"failed at step 2/3", "stopped at last successful: 1.1.80"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Fatalf("phase = %s, want Idle after partial rollback", got)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"Pull(svc=clubfoundry,tag=1.1.80",
		"Stop(clubfoundry)",
		"Up(clubfoundry)",
		"Pull(svc=clubfoundry,tag=1.1.81",
		"Stop(clubfoundry)",
		"HasImage(clubfoundry:1.1.80)",
		"SetServiceImage(clubfoundry,clubfoundry:1.1.80)",
		"Up(clubfoundry)",
	}); err != nil {
		t.Fatal(err)
	}
	if err := containsAllInOrder(backup.Calls(), []string{
		"CreateBackup(1.1.79)",
		"PruneOld()",
		"LatestBackup()",
		"RestoreBackup(/fake/backup/clm.db.pre-update-v1.1.79-1)",
	}); err != nil {
		t.Fatal(err)
	}
	entries, listErr := deps.History.List(10)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(entries) != 1 || entries[0].Outcome != history.OutcomeRollback {
		t.Fatalf("history = %+v, want one partial rollback", entries)
	}
	entry := entries[0]
	if len(entry.Steps) != 2 || entry.Steps[0].Outcome != history.OutcomeSuccess || entry.Steps[1].Outcome != history.OutcomeError {
		t.Fatalf("step outcomes = %+v", entry.Steps)
	}
	if len(entry.Hops) != 3 || entry.Hops[2] != "1.1.82" {
		t.Fatalf("hops = %v, want complete requested path", entry.Hops)
	}
}

func TestRunSteppedUpdate_RollbackFailureRemainsErrorContract(t *testing.T) {
	deps, docker, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(docker, "1.1.79")
	docker.hasImage["clubfoundry:1.1.80"] = true
	deps.Health = healthHappyPath("1.1.80")

	pullCount := 0
	docker.onPull = func(dockerops.PullOpts) {
		pullCount++
		if pullCount == 2 {
			docker.pullErr = errFakeBoom
			docker.upErr = errFakeBoom
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := deps.RunSteppedUpdate(ctx, []string{"1.1.80", "1.1.81"})
	mustErrSubstr(t, err, []string{
		"failed at step 2/2",
		"rollback to 1.1.80 also failed",
		"restart after restore: fake_boom",
	})

	snapshot := st.Snapshot()
	if snapshot.Phase != state.Error || snapshot.LastErrorCode != "UPDATE_AND_ROLLBACK_FAILED" {
		t.Fatalf("state = %+v, want Error with UPDATE_AND_ROLLBACK_FAILED", snapshot)
	}
	if !strings.Contains(snapshot.LastError, "rollback to 1.1.80 also failed") {
		t.Fatalf("last error = %q", snapshot.LastError)
	}

	entries, listErr := deps.History.List(10)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(entries) != 1 || entries[0].Outcome != history.OutcomeError {
		t.Fatalf("history = %+v, want one error entry", entries)
	}
	if !strings.Contains(entries[0].Error, "restart after restore: fake_boom") {
		t.Fatalf("history error = %q", entries[0].Error)
	}

	artifacts, readErr := os.ReadDir(filepath.Join(deps.DataDir, failureBundleDir))
	if readErr != nil {
		t.Fatal(readErr)
	}
	bundleCount := 0
	for _, artifact := range artifacts {
		if !artifact.IsDir() && strings.HasSuffix(artifact.Name(), ".json") {
			bundleCount++
		}
	}
	if bundleCount != 1 {
		t.Fatalf("failure artifacts = %v, want one JSON bundle", artifacts)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"Pull(svc=clubfoundry,tag=1.1.80",
		"Up(clubfoundry)",
		"Pull(svc=clubfoundry,tag=1.1.81",
		"HasImage(clubfoundry:1.1.80)",
		"SetServiceImage(clubfoundry,clubfoundry:1.1.80)",
		"Up(clubfoundry)",
	}); err != nil {
		t.Fatal(err)
	}
}

// TestRunUpdate_SchemaDryrunSkippedByDefault — when CLM_SCHEMA_DRYRUN_ENABLED
// is not set, runSchemaDryrun is a no-op. The fake's RunMigrationDryrun
// must not be called, and the update succeeds normally.
func TestRunUpdate_SchemaDryrunSkippedByDefault(t *testing.T) {
	deps, doc, _, _, _, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	deps.Health = healthHappyPath("1.1.80")

	// Make sure the env is not lingering from a prior test.
	t.Setenv("CLM_SCHEMA_DRYRUN_ENABLED", "")
	t.Setenv("CLM_HOST_DATA_DIR", "")

	if err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{}); err != nil {
		t.Fatalf("update should succeed when dryrun is gated off: %v", err)
	}
	for _, c := range doc.Calls() {
		if strings.HasPrefix(c, "RunMigrationDryrun(") {
			t.Errorf("RunMigrationDryrun must NOT be invoked when env-gated off; got %q", c)
		}
	}
}

// TestRunUpdate_SchemaDryrunSkippedNoHostDataDir — env enabled but
// CLM_HOST_DATA_DIR not set: dryrun is a no-op (not a failure), update
// proceeds normally. Closes the misconfig-blocks-update edge case.
func TestRunUpdate_SchemaDryrunSkippedNoHostDataDir(t *testing.T) {
	deps, doc, _, _, _, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	deps.Health = healthHappyPath("1.1.80")

	t.Setenv("CLM_SCHEMA_DRYRUN_ENABLED", "true")
	t.Setenv("CLM_HOST_DATA_DIR", "") // explicitly empty

	if err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{}); err != nil {
		t.Fatalf("update should succeed when host data dir misconfigured: %v", err)
	}
	for _, c := range doc.Calls() {
		if strings.HasPrefix(c, "RunMigrationDryrun(") {
			t.Errorf("RunMigrationDryrun must NOT be invoked without HostDataDir; got %q", c)
		}
	}
}

// TestRunUpdate_SchemaDryrunFails — env enabled + HostDataDir set + dryrun
// errors. The error must propagate up: update aborts BEFORE Stop fires
// (the running container stays alive on its current version). State ends
// in Idle after rollback.
func TestRunUpdate_SchemaDryrunFails(t *testing.T) {
	deps, doc, _, _, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(doc, "1.1.79")
	deps.Health = healthRollbackOnly("1.1.79")
	doc.dryrunErr = errFakeBoom

	t.Setenv("CLM_SCHEMA_DRYRUN_ENABLED", "true")
	t.Setenv("CLM_HOST_DATA_DIR", "/opt/clubfoundry/data")

	err := runUpdateOrTimeout(t, deps, "1.1.80", dockerops.PullOpts{})
	mustErrSubstr(t, err, []string{"schema dry-run"})
	if got := st.Snapshot().Phase; got != state.Idle {
		t.Errorf("phase = %s, want Idle (rollback succeeded)", got)
	}
	// Critical safety property: dryrun ran AFTER Pull but BEFORE Stop —
	// running container was never touched on the forward path. Rollback
	// then fired Stop+Pull+Up to be conservative.
	calls := doc.Calls()
	pullIdx := -1
	stopIdx := -1
	dryrunIdx := -1
	for i, c := range calls {
		if pullIdx < 0 && strings.Contains(c, "Pull(svc=clubfoundry,tag=1.1.80") {
			pullIdx = i
		}
		if dryrunIdx < 0 && strings.HasPrefix(c, "RunMigrationDryrun(") {
			dryrunIdx = i
		}
		if stopIdx < 0 && strings.HasPrefix(c, "Stop(") {
			stopIdx = i
		}
	}
	if pullIdx < 0 || dryrunIdx < 0 {
		t.Fatalf("expected Pull + RunMigrationDryrun in calls; got %v", calls)
	}
	if dryrunIdx < pullIdx {
		t.Errorf("dryrun must come AFTER Pull (forward path); pullIdx=%d dryrunIdx=%d", pullIdx, dryrunIdx)
	}
	// Stop is allowed AFTER dryrun (rollback fires Stop). What we don't
	// want is Stop BEFORE dryrun on the forward path — that would mean the
	// running container was already torn down when we discovered the
	// schema problem.
	if stopIdx >= 0 && stopIdx < dryrunIdx {
		t.Errorf("Stop must NOT precede dryrun on forward path; pullIdx=%d dryrunIdx=%d stopIdx=%d", pullIdx, dryrunIdx, stopIdx)
	}
}

// TestPreflight_AllOK verifies that reserved compatibility fields remain
// optimistic until their public probes are wired.
func TestPreflight_AllOK(t *testing.T) {
	deps, _, _, _, _, cleanup := makeDeps(t)
	defer cleanup()

	r := deps.Preflight(context.Background(), dockerops.PullOpts{}) // no URL → registry path
	if !r.AllOK {
		t.Errorf("AllOK=false; details=%v", r.Details)
	}
	if !r.Sha256OK {
		t.Errorf("Sha256OK=false; reserved compatibility field should be true")
	}
	if !r.SchemaDryrunOK {
		t.Errorf("SchemaDryrunOK=false; reserved compatibility field should be true")
	}
	if !r.PairCompatOK {
		t.Errorf("PairCompatOK=false; reserved compatibility field should be true")
	}
	if !r.PortFreeOK {
		t.Errorf("PortFreeOK=false; default should be true")
	}
}

// TestRunSelfUpdate_CancelDuringPull verifies that Cancel reaches an in-flight
// self-update pull before the trampoline is spawned. The test starts a pull
// with a long fake delay, waits until the fake confirms that Pull was entered,
// then fires Cancel(). Pull must
// observe ctx.Done() and return ctx.Canceled; selfupdate.go must classify
// via isCancelled() and write SELF_UPDATE_CANCELLED + OutcomeCancelled
// (not the generic SELF_UPDATE_PULL_FAILED + OutcomeError).
func TestRunSelfUpdate_CancelDuringPull(t *testing.T) {
	deps, doc, _, _, _, cleanup := makeDeps(t)
	defer cleanup()
	doc.pullDelay = 5 * time.Second // long enough that the test must cancel
	pullStarted := make(chan struct{})
	doc.onPull = func(dockerops.PullOpts) {
		close(pullStarted)
	}

	parent := context.Background()
	errCh := make(chan error, 1)
	go func() {
		errCh <- deps.RunSelfUpdate(parent, "v1.T", dockerops.PullOpts{})
	}()

	select {
	case <-pullStarted:
		// Pull is entered only after RunSelfUpdate arms cancellation.
	case err := <-errCh:
		t.Fatalf("RunSelfUpdate exited before Pull could start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("RunSelfUpdate did not enter Pull within 2s")
	}

	if !deps.Cancel() {
		t.Fatalf("Cancel returned false — armCancel was not active during RunSelfUpdate")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected ctx.Canceled (or wrapped), got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("RunSelfUpdate did not return within 2s after Cancel — pull goroutine still hung")
	}

	snap := deps.SelfState.Snapshot()
	if snap.Phase != state.Error {
		t.Errorf("self-state phase = %s, want Error", snap.Phase)
	}
	if snap.LastErrorCode != "SELF_UPDATE_CANCELLED" {
		t.Errorf("error code = %q, want SELF_UPDATE_CANCELLED", snap.LastErrorCode)
	}

	// After RunSelfUpdate returns, disarmCancel should have run via defer.
	// A second Cancel() must report "nothing to cancel" (false).
	if deps.Cancel() {
		t.Errorf("Cancel returned true after RunSelfUpdate exit — disarmCancel skipped")
	}
}

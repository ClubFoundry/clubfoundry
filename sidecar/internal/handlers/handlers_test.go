package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/state"
)

func TestHealth(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), Version: "v-test"})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status: got %q, want ok", body["status"])
	}
	if body["version"] != "v-test" {
		t.Errorf("version: got %q, want v-test", body["version"])
	}
}

func TestStatus(t *testing.T) {
	st := state.New()
	mux := http.NewServeMux()
	Register(mux, Deps{State: st, Version: "v-test"})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"phase":"idle"`) {
		t.Errorf("status body should report idle phase, got %s", rr.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), Version: "v-test"})

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestUpdateWhenNoUpdaterReturns503(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), Version: "v-test", Updater: nil})

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when updater nil, got %d", rr.Code)
	}
}

// mockCloud stands up a local HTTP server that returns a canned
// `/api/update` response, then hands back a cloud.Client pointed at it.
func mockCloud(t *testing.T, payload map[string]any) (*cloud.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/update" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	return &cloud.Client{
		BaseURL: srv.URL,
		HTTP:    &http.Client{Timeout: 2 * time.Second},
	}, srv.Close
}

// slowCloud returns a cloud.Client whose server delays the response past
// the sidecar's updateDecisionTimeout. Used to verify fallback behavior.
func slowCloud(t *testing.T) (*cloud.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(updateDecisionTimeout + 500*time.Millisecond)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	return &cloud.Client{
		BaseURL: srv.URL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}, srv.Close
}

// TestResolveUpdateDispatch_AirGapped confirms that when no cloud client is
// configured, the handler falls through to single-step RunUpdate with the
// caller's target (matches pre-stepped-support behavior).
func TestResolveUpdateDispatch_AirGapped(t *testing.T) {
	disp, err := resolveUpdateDispatch(context.Background(), Deps{}, "1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disp.Path != nil {
		t.Errorf("expected nil path (single-step), got %v", disp.Path)
	}
	if disp.Target != "1.2.3" {
		t.Errorf("target: got %q, want 1.2.3", disp.Target)
	}
}

// TestResolveUpdateDispatch_SingleStepDirectJumpOK — cloud documents a single-
// hop path and allows direct jump. Handler picks RunUpdate with path[0].
func TestResolveUpdateDispatch_SingleStepDirectJumpOK(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest":          "1.0.35",
		"currentIsLatest": false,
		"updatePath": map[string]any{
			"from":              "1.0.34",
			"to":                "1.0.35",
			"path":              []string{"1.0.35"},
			"directJumpAllowed": true,
		},
	})
	defer cleanup()

	disp, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disp.Path != nil {
		t.Errorf("expected nil path for single-hop direct jump, got %v", disp.Path)
	}
	if disp.Target != "1.0.35" {
		t.Errorf("target: got %q, want 1.0.35", disp.Target)
	}
}

// TestResolveUpdateDispatch_SingleStepDirectJumpForbidden — even a one-hop
// path must go through the stepped flow when directJumpAllowed=false. This
// is the "guarded single step" case — same migration gate but with the
// explicit stepped machinery engaged.
func TestResolveUpdateDispatch_SingleStepDirectJumpForbidden(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest": "1.0.35",
		"updatePath": map[string]any{
			"path":              []string{"1.0.35"},
			"directJumpAllowed": false,
		},
	})
	defer cleanup()

	disp, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disp.Path) != 1 || disp.Path[0] != "1.0.35" {
		t.Errorf("expected stepped with [1.0.35], got %v", disp.Path)
	}
	if disp.Target != "1.0.35" {
		t.Errorf("target: got %q, want 1.0.35", disp.Target)
	}
}

// TestResolveUpdateDispatch_MultiStep — cloud documents a 3-step path. Handler
// picks RunSteppedUpdate with the full path.
func TestResolveUpdateDispatch_MultiStep(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest": "1.0.35",
		"updatePath": map[string]any{
			"path":              []string{"1.0.25", "1.0.30", "1.0.35"},
			"directJumpAllowed": false,
		},
	})
	defer cleanup()

	disp, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disp.Path) != 3 || disp.Path[0] != "1.0.25" || disp.Path[2] != "1.0.35" {
		t.Errorf("expected 3-step path, got %v", disp.Path)
	}
	if disp.Target != "1.0.35" {
		t.Errorf("target: got %q, want 1.0.35 (final path hop)", disp.Target)
	}
}

// TestResolveUpdateDispatch_TargetTruncatesPath — caller asks for an
// intermediate version from the cloud's documented path. Handler truncates
// the path at that hop and runs stepped up to there.
func TestResolveUpdateDispatch_TargetTruncatesPath(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest": "1.0.35",
		"updatePath": map[string]any{
			"path":              []string{"1.0.25", "1.0.30", "1.0.35"},
			"directJumpAllowed": false,
		},
	})
	defer cleanup()

	disp, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.0.30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disp.Path) != 2 || disp.Path[0] != "1.0.25" || disp.Path[1] != "1.0.30" {
		t.Errorf("expected truncated 2-step path, got %v", disp.Path)
	}
	if disp.Target != "1.0.30" {
		t.Errorf("target: got %q, want 1.0.30", disp.Target)
	}
}

// TestResolveUpdateDispatch_RejectsOutOfPathTarget — caller asks for a
// version that isn't on the cloud-documented path. Must return an error so
// the handler returns 400 instead of silently running a direct jump.
func TestResolveUpdateDispatch_RejectsOutOfPathTarget(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest": "1.0.35",
		"updatePath": map[string]any{
			"path":              []string{"1.0.25", "1.0.30", "1.0.35"},
			"directJumpAllowed": false,
		},
	})
	defer cleanup()

	_, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.0.99")
	if err == nil {
		t.Fatalf("expected error for out-of-path target, got nil")
	}
	if !strings.Contains(err.Error(), "1.0.99") || !strings.Contains(err.Error(), "not in the cloud-documented update path") {
		t.Errorf("error should name the rejected version + path context, got: %v", err)
	}
}

// A non-latest target cannot use the artifact published for channel latest.
func TestResolveUpdateDispatch_BUG019_NoPathNonLatestTarget(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest":         "1.0.35",
		"downloadUrl":    "https://example/clubfoundry-1.0.35.tar.zst",
		"downloadSha256": "deadbeef",
		// No update path is advertised.
	})
	defer cleanup()

	_, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.0.30")
	if err == nil {
		t.Fatalf("expected error for non-latest target with no updatePath, got nil")
	}
	if !strings.Contains(err.Error(), "1.0.30") || !strings.Contains(err.Error(), "1.0.35") {
		t.Errorf("error should name the rejected target + latest, got: %v", err)
	}
}

// An empty advertised path has the same safety contract as a missing path.
func TestResolveUpdateDispatch_BUG019_EmptyPathNonLatestTarget(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest":         "1.0.35",
		"downloadUrl":    "https://example/clubfoundry-1.0.35.tar.zst",
		"downloadSha256": "deadbeef",
		"updatePath": map[string]any{
			"path":              []string{},
			"directJumpAllowed": true,
		},
	})
	defer cleanup()

	_, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.0.30")
	if err == nil {
		t.Fatalf("expected error for non-latest target with empty updatePath, got nil")
	}
	if !strings.Contains(err.Error(), "1.0.30") {
		t.Errorf("error should name the rejected target, got: %v", err)
	}
}

// Direct jumps cannot attach the latest artifact to an intermediate target.
func TestResolveUpdateDispatch_BUG019_DirectJumpIntermediate(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest":         "1.0.35",
		"downloadUrl":    "https://example/clubfoundry-1.0.35.tar.zst",
		"downloadSha256": "deadbeef",
		"updatePath": map[string]any{
			"path":              []string{"1.0.25", "1.0.30", "1.0.35"},
			"directJumpAllowed": true,
		},
	})
	defer cleanup()

	_, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.0.25")
	if err == nil {
		t.Fatalf("expected error for direct-jump-to-intermediate, got nil")
	}
	if !strings.Contains(err.Error(), "1.0.25") || !strings.Contains(err.Error(), "1.0.35") {
		t.Errorf("error should name the rejected intermediate + latest, got: %v", err)
	}
}

// The channel-latest target still receives its verified artifact metadata.
func TestResolveUpdateDispatch_BUG019_LatestRequestStillWorks(t *testing.T) {
	c, cleanup := mockCloud(t, map[string]any{
		"latest":         "1.0.35",
		"downloadUrl":    "https://example/clubfoundry-1.0.35.tar.zst",
		"downloadSha256": "deadbeef",
		"updatePath": map[string]any{
			"path":              []string{"1.0.35"},
			"directJumpAllowed": true,
		},
	})
	defer cleanup()

	disp, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.0.35")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disp.Target != "1.0.35" {
		t.Errorf("target: got %q, want 1.0.35", disp.Target)
	}
	if disp.MainPull.URL != "https://example/clubfoundry-1.0.35.tar.zst" {
		t.Errorf("mainPull URL not stamped for latest=target, got %q", disp.MainPull.URL)
	}
}

// TestResolveUpdateDispatch_CloudTimeoutFallsBackToSingleStep — if the cloud
// is unreachable / slow, the handler falls back to legacy single-step to
// avoid blocking operator-triggered updates. Air-gapped installs still work.
func TestResolveUpdateDispatch_CloudTimeoutFallsBackToSingleStep(t *testing.T) {
	c, cleanup := slowCloud(t)
	defer cleanup()

	start := time.Now()
	disp, err := resolveUpdateDispatch(context.Background(), Deps{Cloud: c}, "1.2.3")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disp.Path != nil {
		t.Errorf("expected single-step fallback, got stepped %v", disp.Path)
	}
	if disp.Target != "1.2.3" {
		t.Errorf("target: got %q, want 1.2.3 (fallback)", disp.Target)
	}
	// Decision timeout caps the fallback at updateDecisionTimeout + some slop.
	if elapsed > updateDecisionTimeout+2*time.Second {
		t.Errorf("fallback took %v, expected ≤%v", elapsed, updateDecisionTimeout+2*time.Second)
	}
}

// TestStatusComposesUnion verifies that /status returns compatibility fields
// plus main_op and self_op sub-objects. Top-level state mirrors whichever
// kind is non-Idle (preferring main).
func TestStatusComposesUnion(t *testing.T) {
	mainSt := state.NewKindAware(state.KindMain, "")
	selfSt := state.NewKindAware(state.KindSelf, "")
	_ = selfSt.TransitionTo(state.Updating, "self updating")
	selfSt.SetTarget("v1.M")

	mux := http.NewServeMux()
	Register(mux, Deps{State: mainSt, SelfState: selfSt, Version: "v-test"})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"main_op":`) {
		t.Errorf("status missing main_op: %s", body)
	}
	if !strings.Contains(body, `"self_op":`) {
		t.Errorf("status missing self_op: %s", body)
	}
	// Main is Idle but self is Updating → top-level should mirror self.
	if !strings.Contains(body, `"phase":"updating"`) {
		t.Errorf("top-level legacy phase should mirror non-Idle kind: %s", body)
	}
}

// TestForceResetRequiresKind — /force-reset without ?kind= must 400.
func TestForceResetRequiresKind(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), SelfState: state.NewKindAware(state.KindSelf, ""), Version: "v-test"})
	req := httptest.NewRequest(http.MethodPost, "/force-reset", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("force-reset without kind: got %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestForceResetSelfClearsState — /force-reset?kind=self wipes selfState
// even when it's Updating (the operator escape).
func TestForceResetSelfClearsState(t *testing.T) {
	selfSt := state.NewKindAware(state.KindSelf, "")
	_ = selfSt.TransitionTo(state.Updating, "stuck self update")
	selfSt.SetTarget("v1.X")

	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), SelfState: selfSt, Version: "v-test"})
	req := httptest.NewRequest(http.MethodPost, "/force-reset?kind=self", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("force-reset?kind=self: got %d body=%s", rr.Code, rr.Body.String())
	}
	if snap := selfSt.Snapshot(); snap.Phase != state.Idle || snap.TargetVersion != "" {
		t.Errorf("force-reset did not clear self state: %+v", snap)
	}
}

// TestResetErrorKindAwareSelf — /reset-error?kind=self resets only selfState.
func TestResetErrorKindAwareSelf(t *testing.T) {
	mainSt := state.New()
	selfSt := state.NewKindAware(state.KindSelf, "")
	mainSt.MarkError("MAIN_ERR", "main has error")
	selfSt.MarkError("SELF_ERR", "self has error")

	mux := http.NewServeMux()
	Register(mux, Deps{State: mainSt, SelfState: selfSt, Version: "v-test"})
	req := httptest.NewRequest(http.MethodPost, "/reset-error?kind=self", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset-error?kind=self: %d body=%s", rr.Code, rr.Body.String())
	}
	if mainSt.Snapshot().Phase != state.Error {
		t.Errorf("main state should still be Error; got %s", mainSt.Snapshot().Phase)
	}
	if selfSt.Snapshot().Phase != state.Idle {
		t.Errorf("self state should be Idle after kind=self reset; got %s", selfSt.Snapshot().Phase)
	}
}

// TestResetErrorWithoutKindResetsAllErrored — legacy contract preserved:
// no kind param, reset every kind currently in Error.
func TestResetErrorWithoutKindResetsAllErrored(t *testing.T) {
	mainSt := state.New()
	selfSt := state.NewKindAware(state.KindSelf, "")
	mainSt.MarkError("MAIN_ERR", "x")
	selfSt.MarkError("SELF_ERR", "y")

	mux := http.NewServeMux()
	Register(mux, Deps{State: mainSt, SelfState: selfSt, Version: "v-test"})
	req := httptest.NewRequest(http.MethodPost, "/reset-error", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reset-error (no kind): %d body=%s", rr.Code, rr.Body.String())
	}
	if mainSt.Snapshot().Phase != state.Idle || selfSt.Snapshot().Phase != state.Idle {
		t.Errorf("both should be Idle after no-kind reset; main=%s self=%s",
			mainSt.Snapshot().Phase, selfSt.Snapshot().Phase)
	}
}

func TestHistoryEmptyWhenNoLog(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), Version: "v-test"})

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Errorf("expected empty array, got %q", rr.Body.String())
	}
}

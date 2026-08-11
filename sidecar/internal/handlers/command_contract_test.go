package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

func TestApplyRequiresCompleteStagedStateContract(t *testing.T) {
	t.Setenv("CLM_UPDATE_MODE", "")

	tests := []struct {
		name    string
		prepare func(*testing.T, *state.State)
		message string
	}{
		{
			name:    "idle",
			prepare: func(*testing.T, *state.State) {},
			message: "not staged (phase=idle)",
		},
		{
			name: "missing target",
			prepare: func(t *testing.T, s *state.State) {
				transitionToStaged(t, s)
			},
			message: "staged target version unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mainState := state.New()
			tc.prepare(t, mainState)
			mux := http.NewServeMux()
			Register(mux, Deps{
				State:   mainState,
				Updater: &updater.Deps{State: mainState},
			})

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/update/apply", nil))
			assertJSONError(t, rr, http.StatusConflict, tc.message)
		})
	}
}

func TestDropStagedContract(t *testing.T) {
	mainState := state.New()
	deps := &updater.Deps{State: mainState}
	mux := http.NewServeMux()
	Register(mux, Deps{State: mainState, Updater: deps})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/update/staged", nil))
	assertJSONError(t, rr, http.StatusConflict, "nothing staged (phase=idle)")

	transitionToStaged(t, mainState)
	mainState.SetStagedTarget("1.2.3")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/update/staged", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "dropped" {
		t.Fatalf("status body = %q, want dropped", body["status"])
	}
	if snap := mainState.Snapshot(); snap.Phase != state.Idle || snap.StagedTarget != "" {
		t.Fatalf("state after drop = %+v, want idle without staged target", snap)
	}
}

func transitionToStaged(t *testing.T, target *state.State) {
	t.Helper()
	if err := target.TransitionTo(state.Staging, "staging"); err != nil {
		t.Fatal(err)
	}
	if err := target.TransitionTo(state.Staged, "staged"); err != nil {
		t.Fatal(err)
	}
}

func TestCancelWithoutActiveOperationContract(t *testing.T) {
	mainState := state.New()
	mux := http.NewServeMux()
	Register(mux, Deps{
		State:   mainState,
		Updater: &updater.Deps{State: mainState},
	})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/cancel", nil))
	assertJSONError(t, rr, http.StatusConflict, "no in-flight operation to cancel")
}

func TestRecoveryValidationContract(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		deps    Deps
		status  int
		message string
	}{
		{
			name:    "reset invalid kind",
			path:    "/reset-error?kind=worker",
			deps:    Deps{State: state.New()},
			status:  http.StatusBadRequest,
			message: `invalid kind="worker" (must be main or self)`,
		},
		{
			name:    "reset main not errored",
			path:    "/reset-error?kind=main",
			deps:    Deps{State: state.New()},
			status:  http.StatusConflict,
			message: "kind=main not in error (phase=idle)",
		},
		{
			name:    "reset self unavailable",
			path:    "/reset-error?kind=self",
			deps:    Deps{State: state.New()},
			status:  http.StatusServiceUnavailable,
			message: "self state not initialized",
		},
		{
			name:    "force reset state unavailable",
			path:    "/force-reset?kind=self",
			deps:    Deps{State: state.New()},
			status:  http.StatusServiceUnavailable,
			message: "kind=self state not initialized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, tc.deps)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, tc.path, nil))
			assertJSONError(t, rr, tc.status, tc.message)
		})
	}
}

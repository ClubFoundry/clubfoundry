package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clubfoundry/updater/internal/config"
	"github.com/clubfoundry/updater/internal/state"
	"github.com/clubfoundry/updater/internal/updater"
)

func TestHTTPMethodContract(t *testing.T) {
	tests := []struct {
		path    string
		method  string
		message string
	}{
		{"/diagnostic-bundle", http.MethodPost, "GET only"},
		{"/footprint", http.MethodPost, "GET only"},
		{"/health", http.MethodPost, "GET only"},
		{"/status", http.MethodPost, "GET only"},
		{"/update", http.MethodGet, "POST only"},
		{"/update/stage", http.MethodGet, "POST only"},
		{"/update/apply", http.MethodGet, "POST only"},
		{"/update/staged", http.MethodGet, "DELETE only"},
		{"/cancel", http.MethodGet, "POST only"},
		{"/self-update", http.MethodGet, "POST only"},
		{"/preflight", http.MethodGet, "POST only"},
		{"/reset-error", http.MethodGet, "POST only"},
		{"/force-reset", http.MethodGet, "POST only"},
		{"/rollback", http.MethodGet, "POST only"},
		{"/failure-bundles", http.MethodPost, "GET only"},
		{"/failure-bundles/example.json", http.MethodPost, "GET or DELETE"},
		{"/history", http.MethodPost, "GET only"},
		{"/log-tail", http.MethodPost, "GET only"},
		{"/config", http.MethodPost, "GET or PUT"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, Deps{
				State:     state.New(),
				SelfState: state.NewKindAware(state.KindSelf, ""),
				LogDir:    filepath.Join(t.TempDir(), "update-logs"),
			})

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405; body=%s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			var body map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body["error"] != tc.message {
				t.Fatalf("error = %q, want %q", body["error"], tc.message)
			}
		})
	}
}

func TestStatusJSONContract(t *testing.T) {
	mainState := state.NewKindAware(state.KindMain, "")
	selfState := state.NewKindAware(state.KindSelf, "")
	if err := mainState.TransitionTo(state.Updating, "Installing release"); err != nil {
		t.Fatal(err)
	}
	mainState.UpdateSubStep(state.SubStepDownloading, "Downloading image")
	mainState.SetTarget("1.2.3")

	mux := http.NewServeMux()
	Register(mux, Deps{State: mainState, SelfState: selfState, Version: "v2.A"})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	for _, key := range []string{"kind", "phase", "sub_step", "detail", "target_version", "main_op", "self_op"} {
		if _, ok := body[key]; !ok {
			t.Errorf("status contract missing %q", key)
		}
	}
	assertJSONString(t, body["kind"], "main")
	assertJSONString(t, body["phase"], "updating")
	assertJSONString(t, body["sub_step"], "downloading")
	assertJSONString(t, body["target_version"], "1.2.3")

	var self map[string]json.RawMessage
	if err := json.Unmarshal(body["self_op"], &self); err != nil {
		t.Fatalf("decode self_op: %v", err)
	}
	assertJSONString(t, self["kind"], "self")
	assertJSONString(t, self["phase"], "idle")
}

func TestConfigHTTPContractDefaultsAndRoundTrip(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "updater-config.json"))
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), ConfigStore: store})

	get := func() config.Settings {
		t.Helper()
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /config status = %d; body=%s", rr.Code, rr.Body.String())
		}
		var got config.Settings
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatalf("decode config: %v", err)
		}
		return got
	}

	if got := get(); got != config.Defaults() {
		t.Fatalf("missing-file GET = %+v, want defaults %+v", got, config.Defaults())
	}

	want := config.Defaults()
	want.AutoUpdate = false
	want.Channel = "beta"
	want.UpdateWindow = "04:00-06:00"
	want.CheckIntervalSec = 1800
	body, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /config status = %d; body=%s", rr.Code, rr.Body.String())
	}
	if got := get(); got != want {
		t.Fatalf("GET after PUT = %+v, want %+v", got, want)
	}

	invalid := []byte(`{"auto_update":true,"update_window":"04:00-06:00","channel":"nightly","check_interval_sec":1800,"auto_prune_grace_days":1,"auto_prune_keep_versions":2,"auto_prune_buildcache_keep_gb":2,"auto_prune_buildcache_age_days":3}`)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(invalid)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCatalogManagedMutationContract(t *testing.T) {
	t.Setenv("CLM_UPDATE_MODE", "truenas_apps")
	for _, path := range []string{"/update", "/update/stage", "/update/apply", "/self-update", "/rollback"} {
		t.Run(path, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, Deps{State: state.New(), SelfState: state.NewKindAware(state.KindSelf, "")})
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, nil))
			if rr.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409; body=%s", rr.Code, rr.Body.String())
			}
			var body map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != "CATALOG_MANAGED" {
				t.Fatalf("code = %q, want CATALOG_MANAGED", body["code"])
			}
		})
	}
}

func TestUpdateMutationUnavailableContract(t *testing.T) {
	t.Setenv("CLM_UPDATE_MODE", "")
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/update"},
		{http.MethodPost, "/update/stage"},
		{http.MethodPost, "/update/apply"},
		{http.MethodDelete, "/update/staged"},
		{http.MethodPost, "/cancel"},
		{http.MethodPost, "/self-update"},
		{http.MethodPost, "/preflight"},
		{http.MethodPost, "/rollback"},
	}

	for _, tc := range paths {
		t.Run(tc.path, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, Deps{State: state.New(), SelfState: state.NewKindAware(state.KindSelf, "")})
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
			assertJSONError(t, rr, http.StatusServiceUnavailable, "updater not initialized")
		})
	}
}

func TestUpdateMutationBusyContract(t *testing.T) {
	t.Setenv("CLM_UPDATE_MODE", "")
	for _, path := range []string{"/update", "/update/stage", "/self-update"} {
		t.Run(path, func(t *testing.T) {
			mainState := state.NewKindAware(state.KindMain, "")
			if err := mainState.TransitionTo(state.Updating, "busy"); err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			Register(mux, Deps{
				State:     mainState,
				SelfState: state.NewKindAware(state.KindSelf, ""),
				Updater:   &updater.Deps{},
			})
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, nil))
			assertJSONError(t, rr, http.StatusConflict, "busy: kind=main")
		})
	}
}

func TestUpdateMutationInvalidJSONContract(t *testing.T) {
	t.Setenv("CLM_UPDATE_MODE", "")
	for _, path := range []string{"/update", "/update/stage", "/preflight"} {
		t.Run(path, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, Deps{
				State:     state.New(),
				SelfState: state.NewKindAware(state.KindSelf, ""),
				Updater:   &updater.Deps{},
			})
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString("{")))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
			}
			var body map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if got := body["error"]; !strings.HasPrefix(got, "invalid JSON body: ") {
				t.Fatalf("error = %q, want invalid JSON body prefix", got)
			}
		})
	}
}

func assertJSONString(t *testing.T, raw json.RawMessage, want string) {
	t.Helper()
	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode JSON string %s: %v", raw, err)
	}
	if got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
}

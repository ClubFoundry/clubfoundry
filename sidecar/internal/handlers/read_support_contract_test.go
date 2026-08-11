package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/clubfoundry/updater/internal/config"
	"github.com/clubfoundry/updater/internal/state"
)

func TestConfigAvailabilityContract(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New()})

	t.Run("GET returns defaults without store", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
		}
		var got config.Settings
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got != config.Defaults() {
			t.Fatalf("settings = %+v, want defaults %+v", got, config.Defaults())
		}
	})

	t.Run("PUT requires store", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/config", bytes.NewBufferString(`{}`)))
		assertJSONError(t, rr, http.StatusServiceUnavailable, "config not initialized")
	})
}

func TestConfigMalformedJSONContract(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "updater-config.json"))
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), ConfigStore: store})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/config", bytes.NewBufferString("{")))
	assertJSONError(t, rr, http.StatusBadRequest, "invalid JSON body")
}

func TestCatalogManagedNormalizationContract(t *testing.T) {
	for _, value := range []string{"truenas_apps", " TRUENAS_APPS ", "TrueNas_Apps"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CLM_UPDATE_MODE", value)
			if !catalogManaged() {
				t.Fatalf("catalogManaged() = false for %q", value)
			}
		})
	}

	for _, value := range []string{"", "standalone", "truenas-apps"} {
		t.Run("not_"+value, func(t *testing.T) {
			t.Setenv("CLM_UPDATE_MODE", value)
			if catalogManaged() {
				t.Fatalf("catalogManaged() = true for %q", value)
			}
		})
	}
}

func TestSafeVersionTokenContract(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"1.3.138", true},
		{"release_candidate-1:amd64", true},
		{"", false},
		{"1.3.138 latest", false},
		{"1.3.138;reboot", false},
		{"$(command)", false},
		{"v/1", false},
		{string(bytes.Repeat([]byte("a"), 200)), true},
		{string(bytes.Repeat([]byte("a"), 201)), false},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			if got := isSafeVersionToken(tc.value); got != tc.want {
				t.Fatalf("isSafeVersionToken(%q) = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}

func TestBusyKindPriorityContract(t *testing.T) {
	mainState := state.NewKindAware(state.KindMain, "")
	selfState := state.NewKindAware(state.KindSelf, "")
	if err := mainState.TransitionTo(state.Updating, "main"); err != nil {
		t.Fatal(err)
	}
	if err := selfState.TransitionTo(state.Updating, "self"); err != nil {
		t.Fatal(err)
	}

	kind, busy := anyBusy(mainState, selfState)
	if !busy || kind != state.KindMain {
		t.Fatalf("anyBusy() = (%q, %t), want (main, true)", kind, busy)
	}
}

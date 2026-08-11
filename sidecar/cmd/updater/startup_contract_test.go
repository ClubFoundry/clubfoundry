package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/state"
)

func TestRuntimeSettingsContract(t *testing.T) {
	t.Setenv("CLUBFOUNDRY_UPDATER_ADDR", "")
	t.Setenv("CLUBFOUNDRY_DATA_DIR", "")
	if got := loadRuntimeSettings(); got != (runtimeSettings{Addr: ":3001", DataDir: "/app/data"}) {
		t.Fatalf("default settings = %+v", got)
	}

	t.Setenv("CLUBFOUNDRY_UPDATER_ADDR", "127.0.0.1:9301")
	t.Setenv("CLUBFOUNDRY_DATA_DIR", "/srv/clubfoundry")
	if got := loadRuntimeSettings(); got != (runtimeSettings{Addr: "127.0.0.1:9301", DataDir: "/srv/clubfoundry"}) {
		t.Fatalf("overridden settings = %+v", got)
	}
}

func TestBackupConfigDataDirContract(t *testing.T) {
	t.Setenv("CLUBFOUNDRY_DB_PATH", "/custom/db.sqlite")
	t.Setenv("CLUBFOUNDRY_BACKUPS_DIR", "/custom/backups")

	standard := backupConfigForDataDir("/app/data")
	if standard.DBPath != "/custom/db.sqlite" || standard.BackupsDir != "/custom/backups" || standard.KeepN != 5 {
		t.Fatalf("standard config = %+v", standard)
	}

	custom := backupConfigForDataDir("/srv/clubfoundry")
	if custom.DBPath != filepath.Join("/srv/clubfoundry", "clm.db") || custom.BackupsDir != filepath.Join("/srv/clubfoundry", "backups") || custom.KeepN != 5 {
		t.Fatalf("custom data-dir config = %+v", custom)
	}
}

func TestTelemetrySelectionContract(t *testing.T) {
	healthCheck := &health.Checker{URL: "http://main/health"}
	if got := telemetryForCloud(nil, healthCheck, "/data"); got != nil {
		t.Fatalf("nil cloud reporter = %#v, want nil", got)
	}
	if got := telemetryForCloud(&cloud.Client{}, healthCheck, "/data"); got != nil {
		t.Fatalf("offline reporter = %#v, want nil", got)
	}

	t.Setenv("CLUBFOUNDRY_INSTANCE_ID", "instance-test")
	got := telemetryForCloud(&cloud.Client{BaseURL: "https://cloud.example"}, healthCheck, "/data")
	if got == nil {
		t.Fatal("online reporter = nil")
	}
	if got.CloudBaseURL != "https://cloud.example" || got.InstanceID != "instance-test" || got.Health != healthCheck || got.LogDir != filepath.Join("/data", "update-logs") {
		t.Fatalf("online reporter = %+v", got)
	}
}

func TestHTTPServerContract(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := newHTTPServer("127.0.0.1:9301", handler)
	if srv.Addr != "127.0.0.1:9301" || srv.ReadHeaderTimeout != 10*time.Second || srv.ReadTimeout != 30*time.Second || srv.WriteTimeout != 30*time.Second || srv.IdleTimeout != 120*time.Second {
		t.Fatalf("server settings = %+v", srv)
	}
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("handler status = %d, want 204", rr.Code)
	}
}

func TestRuntimeDependencyWiringContract(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("CLUBFOUNDRY_DATA_DIR", dataDir)
	t.Setenv("CLUBFOUNDRY_CLOUD_URL", "")
	t.Setenv("CLUBFOUNDRY_COMPOSE_DIR", "/compose-test")
	t.Setenv("CLUBFOUNDRY_MAIN_SERVICE", "main-test")
	t.Setenv("CLUBFOUNDRY_UPDATER_SERVICE", "updater-test")

	mainState := state.NewKindAware(state.KindMain, "")
	selfState := state.NewKindAware(state.KindSelf, "")
	runtime := initializeRuntimeDependencies(dataDir, "v-test", mainState, selfState)

	if runtime.docker.ComposeDir != "/compose-test" || runtime.docker.MainServiceName() != "main-test" || runtime.docker.UpdaterServiceName() != "updater-test" {
		t.Fatalf("docker wiring = %+v", runtime.docker)
	}
	if runtime.updater.State != mainState || runtime.updater.SelfState != selfState {
		t.Fatal("updater state wiring does not preserve state instances")
	}
	if runtime.updater.History != runtime.history || runtime.updater.Health != runtime.health {
		t.Fatal("updater collaborators do not preserve runtime instances")
	}
	if runtime.updater.DataDir != dataDir || runtime.updater.LogDir != filepath.Join(dataDir, "update-logs") || runtime.updater.SelfVersion != "v-test" {
		t.Fatalf("updater paths/version = data:%q log:%q version:%q", runtime.updater.DataDir, runtime.updater.LogDir, runtime.updater.SelfVersion)
	}
	if runtime.updater.Telemetry != nil {
		t.Fatalf("offline telemetry = %#v, want nil", runtime.updater.Telemetry)
	}
}

func TestRuntimeRouteWiringContract(t *testing.T) {
	mainState := state.NewKindAware(state.KindMain, "")
	selfState := state.NewKindAware(state.KindSelf, "")
	runtime := runtimeDependencies{}
	mux, events := registerRuntimeRoutes(t.TempDir(), "v-route", runtime, mainState, selfState)
	if events == nil {
		t.Fatal("recovery event store = nil")
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "{\"status\":\"ok\",\"version\":\"v-route\"}\n" {
		t.Fatalf("health response = status:%d body:%q", rr.Code, rr.Body.String())
	}
}

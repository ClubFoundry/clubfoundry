package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/history"
)

func TestRunReportContract(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.3.138"}`))
	}))
	defer healthServer.Close()

	var got UpdateReport
	var gotMethod, gotPath, gotContentType, gotUserAgent string
	reportServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotUserAgent = r.Header.Get("User-Agent")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode report: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer reportServer.Close()

	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "update-1.log"), []byte("downloaded\nhealthy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	entry := history.Entry{
		ID:          "update-1",
		StartedAt:   started,
		FinishedAt:  started.Add(45 * time.Second),
		DurationMS:  45_000,
		FromVersion: "1.3.137",
		ToVersion:   "1.3.138",
		Outcome:     history.OutcomeSuccess,
		Steps: []history.Step{{
			From: "1.3.137", To: "1.3.138", Outcome: history.OutcomeSuccess, Duration: 44_000,
		}},
	}
	r := &Reporter{
		CloudBaseURL: reportServer.URL,
		InstanceID:   "instance-1",
		Health:       &health.Checker{URL: healthServer.URL, Client: healthServer.Client()},
		LogDir:       logDir,
		HTTPClient:   reportServer.Client(),
		SettleWindow: time.Nanosecond,
	}

	r.runReport(entry)

	if gotMethod != http.MethodPost || gotPath != reportEndpointPath {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotContentType != "application/json" || gotUserAgent != userAgent {
		t.Fatalf("headers = content-type %q user-agent %q", gotContentType, gotUserAgent)
	}
	if got.ReportVersion != 1 || got.InstanceID != "instance-1" || got.UpdateID != entry.ID {
		t.Fatalf("identity fields = %+v", got)
	}
	if got.FromVersion != entry.FromVersion || got.ToVersion != entry.ToVersion || got.Outcome != "success" {
		t.Fatalf("update fields = %+v", got)
	}
	if got.StartedAt != entry.StartedAt.Format(time.RFC3339) || got.FinishedAt != entry.FinishedAt.Format(time.RFC3339) {
		t.Fatalf("timestamps = %+v", got)
	}
	if got.SettleResult != "healthy" || got.SettleAfterSec != 0 || got.Log != "downloaded\nhealthy\n" {
		t.Fatalf("settle/log fields = %+v", got)
	}
	if len(got.Steps) != 1 || got.Steps[0].Duration != 44_000 {
		t.Fatalf("steps = %+v", got.Steps)
	}
}

func TestReporterDefaultsAndLogTailContract(t *testing.T) {
	r := &Reporter{}
	if r.settleWindow() != defaultSettleWindow {
		t.Fatalf("default settle window = %s", r.settleWindow())
	}
	if r.logDir() != "/app/data/update-logs" {
		t.Fatalf("default log dir = %q", r.logDir())
	}
	if r.readLog("missing") != "" {
		t.Fatal("missing log must return an empty string")
	}

	logDir := t.TempDir()
	body := strings.Repeat("a", 33*1024)
	if err := os.WriteFile(filepath.Join(logDir, "large.log"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r.LogDir = logDir
	got := r.readLog("large")
	const marker = "...[truncated]...\n"
	if !strings.HasPrefix(got, marker) || len(got) != len(marker)+32*1024 {
		t.Fatalf("truncated log length/prefix mismatch: len=%d", len(got))
	}
}

func TestCheckSettleHealthContract(t *testing.T) {
	if got := (&Reporter{}).checkSettleHealth(context.Background(), "1.3.138"); got != "unknown" {
		t.Fatalf("nil checker = %q", got)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.3.139"}`))
	}))
	r := &Reporter{Health: &health.Checker{URL: server.URL, Client: server.Client()}}
	if got := r.checkSettleHealth(context.Background(), "1.3.138"); got != "version_mismatch" {
		server.Close()
		t.Fatalf("version mismatch = %q", got)
	}
	server.Close()
	if got := r.checkSettleHealth(context.Background(), "1.3.138"); got != "unhealthy" {
		t.Fatalf("failed probe = %q", got)
	}
}

func TestDeliverOnceRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("temporary failure"))
	}))
	defer server.Close()
	r := &Reporter{CloudBaseURL: server.URL, HTTPClient: server.Client()}
	if err := r.deliverOnce(context.Background(), []byte(`{}`)); err == nil || err.Error() != "HTTP 502" {
		t.Fatalf("deliverOnce error = %v", err)
	}
}

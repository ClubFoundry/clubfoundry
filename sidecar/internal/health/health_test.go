package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeHTTPContracts(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		wantOK     bool
		wantStatus string
		wantPhase  string
	}{
		{name: "healthy json", statusCode: http.StatusOK, body: `{"status":"ok","version":"1.3.138"}`, wantOK: true, wantStatus: "ok"},
		{name: "starting json", statusCode: http.StatusOK, body: `{"status":"starting","phase":"migrating"}`, wantStatus: "starting", wantPhase: "migrating"},
		{name: "plain text compatibility", statusCode: http.StatusOK, body: "OK", wantOK: true, wantStatus: "ok"},
		{name: "service unavailable", statusCode: http.StatusServiceUnavailable, body: `{"status":"starting"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			checker := &Checker{URL: server.URL, Client: server.Client()}
			ok, report, err := checker.Probe(context.Background())
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if ok != tc.wantOK || report.Status != tc.wantStatus || report.Phase != tc.wantPhase {
				t.Fatalf("Probe = (%v, %+v), want ok=%v status=%q phase=%q", ok, report, tc.wantOK, tc.wantStatus, tc.wantPhase)
			}
		})
	}
}

func TestProbeFallsBackToBootState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	client := server.Client()
	server.Close()

	path := filepath.Join(t.TempDir(), ".boot-state.json")
	if err := os.WriteFile(path, []byte(`{"phase":"migrating","migration":"0042.sql","at":"now"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	checker := &Checker{URL: url, Client: client, BootStatePath: path}
	ok, report, err := checker.Probe(context.Background())
	if err != nil || ok || report.Status != "starting" || report.Phase != "migrating" || report.Migration != "0042.sql" {
		t.Fatalf("Probe fallback = (%v, %+v, %v)", ok, report, err)
	}
}

func TestWaitHealthyProgressAndCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"status":"starting","phase":"migrating"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.3.138"}`))
	}))
	defer server.Close()
	checker := &Checker{URL: server.URL, Client: server.Client()}
	var progress []Report
	report, err := checker.WaitHealthy(context.Background(), time.Millisecond, func(r Report) {
		progress = append(progress, r)
	})
	if err != nil || report.Version != "1.3.138" {
		t.Fatalf("WaitHealthy = (%+v, %v)", report, err)
	}
	if len(progress) != 2 || progress[0].Phase != "migrating" || progress[1].Status != "ok" {
		t.Fatalf("progress = %+v", progress)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = checker.WaitHealthy(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestReadBootStateInvalidInputs(t *testing.T) {
	if got := (&Checker{}).readBootState(); got != (bootState{}) {
		t.Fatalf("empty path = %+v", got)
	}
	path := filepath.Join(t.TempDir(), "boot.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := (&Checker{BootStatePath: path}).readBootState(); got != (bootState{}) {
		t.Fatalf("invalid json = %+v", got)
	}
}

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clubfoundry/updater/internal/state"
)

func TestFailureBundleHTTPContract(t *testing.T) {
	dataDir := t.TempDir()
	logDir := filepath.Join(dataDir, "update-logs")
	bundleDir := filepath.Join(dataDir, "update-failures")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const filename = "failure-upd-1.json"
	const bundle = `{"update_id":"upd-1","from_version":"1.0.0","to_version":"1.0.1","outcome":"rollback","source":"update"}`
	if err := os.WriteFile(filepath.Join(bundleDir, filename), []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "ignored.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), LogDir: logDir})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var summaries []struct {
		Filename    string `json:"filename"`
		SizeBytes   int64  `json:"size_bytes"`
		ModifiedAt  int64  `json:"modified_at"`
		UpdateID    string `json:"update_id"`
		FromVersion string `json:"from_version"`
		ToVersion   string `json:"to_version"`
		Outcome     string `json:"outcome"`
		Source      string `json:"source"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&summaries); err != nil {
		t.Fatalf("decode summaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d, want 1", len(summaries))
	}
	got := summaries[0]
	if got.Filename != filename || got.UpdateID != "upd-1" || got.FromVersion != "1.0.0" || got.ToVersion != "1.0.1" || got.Outcome != "rollback" || got.Source != "update" {
		t.Fatalf("summary = %+v, want bundle headline fields", got)
	}
	if got.SizeBytes != int64(len(bundle)) || got.ModifiedAt <= 0 {
		t.Fatalf("summary metadata = %+v, want size=%d and positive modified_at", got, len(bundle))
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles/"+filename, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var readBack map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&readBack); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if readBack["update_id"] != "upd-1" || readBack["outcome"] != "rollback" {
		t.Fatalf("bundle = %+v, want persisted contents", readBack)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles/..secret.json", nil))
	assertJSONError(t, rr, http.StatusBadRequest, "invalid filename")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles/", nil))
	assertJSONError(t, rr, http.StatusBadRequest, "missing filename")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles/readme.txt", nil))
	assertJSONError(t, rr, http.StatusBadRequest, "must end in .json")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/failure-bundles/"+filename, nil))
	assertJSONStatus(t, rr, http.StatusOK, "deleted")

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/failure-bundles/"+filename, nil))
	assertJSONStatus(t, rr, http.StatusOK, "already_gone")
}

func TestFailureBundleFallbackContract(t *testing.T) {
	t.Run("malformed bundle remains visible", func(t *testing.T) {
		dataDir := t.TempDir()
		logDir := filepath.Join(dataDir, "update-logs")
		bundleDir := filepath.Join(dataDir, "update-failures")
		if err := os.MkdirAll(bundleDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, "malformed.json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}

		mux := http.NewServeMux()
		Register(mux, Deps{State: state.New(), LogDir: logDir})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
		}
		var summaries []failureBundleSummary
		if err := json.NewDecoder(rr.Body).Decode(&summaries); err != nil {
			t.Fatal(err)
		}
		if len(summaries) != 1 || summaries[0].Filename != "malformed.json" || summaries[0].SizeBytes != 1 {
			t.Fatalf("summaries = %+v, want malformed file metadata", summaries)
		}
		if summaries[0].UpdateID != "" || summaries[0].Outcome != "" {
			t.Fatalf("headline = %+v, want empty best-effort fields", summaries[0])
		}
	})

	t.Run("item errors remain stable", func(t *testing.T) {
		mux := http.NewServeMux()
		Register(mux, Deps{State: state.New()})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles/missing.json", nil))
		assertJSONError(t, rr, http.StatusServiceUnavailable, "log dir not configured")

		logDir := filepath.Join(t.TempDir(), "update-logs")
		mux = http.NewServeMux()
		Register(mux, Deps{State: state.New(), LogDir: logDir})
		rr = httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/failure-bundles/missing.json", nil))
		assertJSONError(t, rr, http.StatusNotFound, "bundle not found")
	})
}

func TestLogTailHTTPContract(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "update-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, Deps{State: state.New(), LogDir: logDir})

	t.Run("missing file preserves offset", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/log-tail?update_id=missing&from=7", nil))
		assertLogTail(t, rr, http.StatusOK, "", 7, 7, false)
	})

	t.Run("large file returns bounded tail", func(t *testing.T) {
		prefix := bytes.Repeat([]byte("A"), 1024)
		tail := bytes.Repeat([]byte("B"), logTailMaxBytes)
		body := append(prefix, tail...)
		if err := os.WriteFile(filepath.Join(logDir, "upd-2.log"), body, 0o644); err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/log-tail?update_id=upd-2", nil))
		assertLogTail(t, rr, http.StatusOK, string(tail), int64(len(prefix)), int64(len(body)), true)
	})

	t.Run("rejects traversal", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/log-tail?update_id=..%2Fsecret", nil))
		assertJSONError(t, rr, http.StatusBadRequest, "invalid update_id")
	})

	t.Run("rejects negative offset", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/log-tail?update_id=upd-2&from=-1", nil))
		assertJSONError(t, rr, http.StatusBadRequest, "invalid from offset")
	})

	t.Run("offset past EOF resets after rotation", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(logDir, "rotated.log"), []byte("fresh"), 0o644); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/log-tail?update_id=rotated&from=99", nil))
		assertLogTail(t, rr, http.StatusOK, "fresh", 0, 5, false)
	})
}

func assertLogTail(t *testing.T, rr *httptest.ResponseRecorder, wantCode int, wantContent string, wantOffset, wantNext int64, wantTruncated bool) {
	t.Helper()
	if rr.Code != wantCode {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, wantCode, rr.Body.String())
	}
	var body struct {
		Content    string `json:"content"`
		Offset     int64  `json:"offset"`
		NextOffset int64  `json:"nextOffset"`
		Truncated  bool   `json:"truncated"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode log tail: %v", err)
	}
	if body.Content != wantContent || body.Offset != wantOffset || body.NextOffset != wantNext || body.Truncated != wantTruncated {
		t.Fatalf("log tail = {content-len:%d offset:%d next:%d truncated:%t}, want {content-len:%d offset:%d next:%d truncated:%t}", len(body.Content), body.Offset, body.NextOffset, body.Truncated, len(wantContent), wantOffset, wantNext, wantTruncated)
	}
}

func assertJSONError(t *testing.T, rr *httptest.ResponseRecorder, wantCode int, wantMessage string) {
	t.Helper()
	if rr.Code != wantCode {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, wantCode, rr.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["error"] != wantMessage {
		t.Fatalf("error = %q, want %q", body["error"], wantMessage)
	}
}

func assertJSONStatus(t *testing.T, rr *httptest.ResponseRecorder, wantCode int, wantStatus string) {
	t.Helper()
	if rr.Code != wantCode {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, wantCode, rr.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if strings.TrimSpace(body["status"]) != wantStatus {
		t.Fatalf("status body = %q, want %q", body["status"], wantStatus)
	}
}

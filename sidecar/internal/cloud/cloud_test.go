package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheckUpdatesHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("current") != "1.0.20" {
			t.Errorf("missing current param: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("channel") != "stable" {
			t.Errorf("missing channel param: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("instanceId") != "instance-7" {
			t.Errorf("missing instanceId param: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"latest": "1.0.35",
			"currentIsLatest": false,
			"critical": false,
			"recalled": ["1.0.30"],
			"rollbackTo": "1.0.29",
			"updatePath": {"from":"1.0.20","to":"1.0.35","path":["1.0.25","1.0.30","1.0.35"],"directJumpAllowed":false}
		}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Instance: "instance-7", HTTP: srv.Client()}
	resp, err := c.CheckUpdates(context.Background(), "1.0.20", "stable")
	if err != nil {
		t.Fatalf("CheckUpdates: %v", err)
	}
	if resp.Latest != "1.0.35" {
		t.Errorf("latest: %s", resp.Latest)
	}
	if len(resp.Recalled) != 1 || resp.Recalled[0] != "1.0.30" {
		t.Errorf("recalled: %v", resp.Recalled)
	}
	if resp.UpdatePath == nil || len(resp.UpdatePath.Path) != 3 {
		t.Errorf("path: %+v", resp.UpdatePath)
	}
}

func TestFetchVersionMetadataContract(t *testing.T) {
	const version = "1.3.138+recovery"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.EscapedPath() != "/api/version/"+url.PathEscape(version) {
			t.Errorf("path = %s, want escaped version path", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version":"1.3.138+recovery",
			"channel":"alpha",
			"downloadUrls":["https://downloads.example/main.tar"],
			"downloadSha256":"main-sha",
			"updaterVersion":"v3.138",
			"updaterDownloadUrls":["https://downloads.example/updater.tar"]
		}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := c.FetchVersionMetadata(context.Background(), version)
	if err != nil {
		t.Fatalf("FetchVersionMetadata: %v", err)
	}
	if got.Version != version || got.Channel != "alpha" || got.DownloadSha256 != "main-sha" {
		t.Fatalf("metadata mismatch: %+v", got)
	}
	if len(got.DownloadUrls) != 1 || len(got.UpdaterDownloadUrls) != 1 {
		t.Fatalf("download URLs were not decoded: %+v", got)
	}
}

func TestFetchVersionMetadataInputAndStatusContracts(t *testing.T) {
	t.Run("offline", func(t *testing.T) {
		got, err := (&Client{}).FetchVersionMetadata(context.Background(), "1.3.138")
		if err != nil || got != nil {
			t.Fatalf("offline result = (%+v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("empty version", func(t *testing.T) {
		_, err := (&Client{BaseURL: "https://cloud.example"}).FetchVersionMetadata(context.Background(), "")
		if err == nil || err.Error() != "FetchVersionMetadata: empty version" {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		_, err := (&Client{BaseURL: srv.URL, HTTP: srv.Client()}).FetchVersionMetadata(context.Background(), "1.3.404")
		if err == nil || !strings.Contains(err.Error(), `version "1.3.404" not found`) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPostAlertContract(t *testing.T) {
	type alertBody struct {
		InstanceID string `json:"instance_id"`
		Reason     string `json:"reason"`
		Timestamp  string `json:"ts"`
	}

	var got alertBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/sidecar/alert" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Instance: "instance-9", HTTP: srv.Client()}
	if err := c.PostAlert(context.Background(), `health "failed"`); err != nil {
		t.Fatalf("PostAlert: %v", err)
	}
	if got.InstanceID != "instance-9" || got.Reason != `health "failed"` {
		t.Fatalf("alert body = %+v", got)
	}
	if _, err := time.Parse(time.RFC3339, got.Timestamp); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", got.Timestamp, err)
	}
}

func TestPostAlertOfflineIsNoOp(t *testing.T) {
	if err := (&Client{}).PostAlert(context.Background(), "ignored"); err != nil {
		t.Fatalf("PostAlert offline: %v", err)
	}
}

func TestEmptyBaseURLIsNoOp(t *testing.T) {
	c := &Client{BaseURL: "", HTTP: http.DefaultClient}
	resp, err := c.CheckUpdates(context.Background(), "1.0.20", "stable")
	if err != nil || resp != nil {
		t.Errorf("empty BaseURL should return (nil,nil), got (%v, %v)", resp, err)
	}
}

func TestIsRecalled(t *testing.T) {
	r := &UpdateResponse{Recalled: []string{"1.0.30", "1.0.31"}}
	if !IsRecalled(r, "1.0.30") {
		t.Errorf("expected 1.0.30 recalled")
	}
	if IsRecalled(r, "1.0.32") {
		t.Errorf("1.0.32 not recalled")
	}
	if IsRecalled(nil, "1.0.30") {
		t.Errorf("nil response should never match")
	}
}

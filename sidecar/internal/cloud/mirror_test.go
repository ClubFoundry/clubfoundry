package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsStrictlyNewerAppVersion(t *testing.T) {
	cases := []struct {
		current, advertised string
		want                bool
		why                 string
	}{
		{"1.3.121", "1.3.122", true, "patch bump"},
		{"1.3.122", "1.4.0", true, "minor bump"},
		{"1.3.122", "2.0.0", true, "major bump"},
		{"1.3.122", "1.3.122", false, "equal is not newer"},
		{"1.3.122", "1.3.121", false, "older is not newer"},
		{"1.3.122", "1.2.999", false, "higher patch cannot beat a lower minor"},
		{"1.10.0", "1.9.0", false, "numeric compare, not lexicographic"},
		{"1.9.0", "1.10.0", true, "numeric compare, not lexicographic"},
		// Fail-closed: a manifest we cannot parse must never license an install.
		{"1.3.122", "", false, "empty advertised"},
		{"", "1.3.122", false, "empty current"},
		{"1.3.122", "v1.3.123", false, "v prefix is not a shape we publish"},
		{"1.3.122", "1.3.123-rc1", false, "pre-release is not a shape we publish"},
		{"1.3.122", "1.3", false, "too few components"},
		{"1.3.122", "1.3.122.1", false, "too many components"},
		{"1.3.122", "1.3.abc", false, "non-numeric"},
		{"dev", "1.3.122", false, "unparseable current stays put"},
	}
	for _, c := range cases {
		if got := IsStrictlyNewerAppVersion(c.current, c.advertised); got != c.want {
			t.Errorf("IsStrictlyNewerAppVersion(%q, %q) = %v, want %v — %s",
				c.current, c.advertised, got, c.want, c.why)
		}
	}
}

func manifestServer(t *testing.T, status int, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withHosts swaps the mirror list for the duration of a test.
func withHosts(t *testing.T, hosts ...string) {
	t.Helper()
	prev := channelManifestHosts
	channelManifestHosts = hosts
	t.Cleanup(func() { channelManifestHosts = prev })
}

func goodManifest() string {
	b, _ := json.Marshal(ChannelManifest{
		Channel:        "alpha",
		Latest:         "1.3.122",
		DownloadUrls:   []string{"https://example.invalid/a.tar.gz"},
		DownloadSha256: "abc123",
		UpdaterVersion: "v3.AE",
	})
	return string(b)
}

func TestFetchChannelManifestHappyPath(t *testing.T) {
	srv := manifestServer(t, 200, "application/json", goodManifest())
	withHosts(t, srv.URL)

	m, err := fetchChannelManifest(context.Background(), srv.Client(), "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Latest != "1.3.122" || m.DownloadSha256 != "abc123" {
		t.Fatalf("got %+v", m)
	}
}

// The trap this whole validation path exists for. Object-storage hosts with a
// web-listing/SPA fallback answer 200 + the site's HTML for a MISSING key. A
// status-code-only check reads that as success, the HTML decodes into a
// zero-valued struct, and the sidecar concludes "no update available" —
// forever, silently. That is worse than the outage the fallback is for.
func TestFetchChannelManifestRejectsHtmlFallback(t *testing.T) {
	html := "<!DOCTYPE html>\n<html lang=\"en\"><head><title>ClubFoundry</title></head><body>landing</body></html>"
	srv := manifestServer(t, 200, "text/html", html)
	withHosts(t, srv.URL)

	if _, err := fetchChannelManifest(context.Background(), srv.Client(), "alpha"); err == nil {
		t.Fatal("a 200 full of landing-page HTML must be rejected, not read as 'no update'")
	}
}

// Valid JSON that is simply not our manifest must not pass either — the HTML
// case is only the loudest example of "200 does not mean the object is there".
func TestFetchChannelManifestRejectsEmptyJson(t *testing.T) {
	srv := manifestServer(t, 200, "application/json", "{}")
	withHosts(t, srv.URL)

	if _, err := fetchChannelManifest(context.Background(), srv.Client(), "alpha"); err == nil {
		t.Fatal("JSON with no latest field must be rejected")
	}
}

// No sha256 anchor means the download cannot be verified, and the fallback path
// is the last place to relax that.
func TestFetchChannelManifestRequiresSha(t *testing.T) {
	body := `{"channel":"alpha","latest":"1.3.122"}`
	srv := manifestServer(t, 200, "application/json", body)
	withHosts(t, srv.URL)

	if _, err := fetchChannelManifest(context.Background(), srv.Client(), "alpha"); err == nil {
		t.Fatal("a manifest without downloadSha256 must be rejected")
	}
}

func TestFetchChannelManifestRejectsWrongChannel(t *testing.T) {
	body := `{"channel":"stable","latest":"1.3.122","downloadSha256":"abc"}`
	srv := manifestServer(t, 200, "application/json", body)
	withHosts(t, srv.URL)

	if _, err := fetchChannelManifest(context.Background(), srv.Client(), "alpha"); err == nil {
		t.Fatal("a manifest for another channel must be rejected")
	}
}

// First host down, second good: the whole point of two mirrors.
func TestFetchChannelManifestFallsOverToSecondHost(t *testing.T) {
	dead := manifestServer(t, 500, "text/plain", "boom")
	good := manifestServer(t, 200, "application/json", goodManifest())
	withHosts(t, dead.URL, good.URL)

	m, err := fetchChannelManifest(context.Background(), good.Client(), "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Latest != "1.3.122" {
		t.Fatalf("got %+v", m)
	}
}

// A Worker error must fall back to the channel manifest so a mirror can still
// advertise a newer build.
func TestCheckUpdatesFallsBackWhenWorkerIs500(t *testing.T) {
	worker := manifestServer(t, 500, "application/json", `{"error":"Internal server error"}`)
	mirror := manifestServer(t, 200, "application/json", goodManifest())
	withHosts(t, mirror.URL)

	c := &Client{BaseURL: worker.URL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	resp, err := c.CheckUpdates(context.Background(), "1.3.121", "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Latest != "1.3.122" {
		t.Fatalf("latest = %q, want 1.3.122", resp.Latest)
	}
	if resp.CurrentIsLatest {
		t.Error("1.3.121 is not the latest when the manifest says 1.3.122")
	}
	if resp.Critical {
		t.Error("the fallback must never claim critical — it cannot know")
	}
	if resp.RollbackTo != "" {
		t.Error("the fallback must never direct a rollback — that is the Worker's call")
	}
}

// Same outage, but the install is already current: it must stay put rather than
// reinstall itself off the fallback.
func TestCheckUpdatesFallbackSaysStayPutWhenCurrent(t *testing.T) {
	worker := manifestServer(t, 500, "application/json", `{"error":"Internal server error"}`)
	mirror := manifestServer(t, 200, "application/json", goodManifest())
	withHosts(t, mirror.URL)

	c := &Client{BaseURL: worker.URL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	resp, err := c.CheckUpdates(context.Background(), "1.3.122", "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.CurrentIsLatest {
		t.Error("1.3.122 IS the latest — the fallback must not offer it to itself")
	}
}

// A newer install must not be dragged backwards by the fallback.
func TestCheckUpdatesFallbackNeverDowngrades(t *testing.T) {
	worker := manifestServer(t, 500, "application/json", `{"error":"x"}`)
	mirror := manifestServer(t, 200, "application/json", goodManifest())
	withHosts(t, mirror.URL)

	c := &Client{BaseURL: worker.URL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	resp, err := c.CheckUpdates(context.Background(), "1.4.0", "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.CurrentIsLatest {
		t.Error("1.4.0 is newer than the manifest's 1.3.122 — must report itself latest, not downgrade")
	}
}

// Worker healthy → the mirror is not consulted at all, and the Worker's richer
// answer (critical/rollback/updatePath) survives untouched.
func TestCheckUpdatesPrefersWorkerWhenHealthy(t *testing.T) {
	worker := manifestServer(t, 200, "application/json",
		`{"latest":"1.3.200","currentIsLatest":false,"critical":true}`)
	withHosts(t, "http://127.0.0.1:1") // would fail if touched

	c := &Client{BaseURL: worker.URL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	resp, err := c.CheckUpdates(context.Background(), "1.3.121", "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Latest != "1.3.200" || !resp.Critical {
		t.Fatalf("the Worker's answer must win untouched, got %+v", resp)
	}
}

// Air-gapped installs opt out by leaving the URL empty; that is a choice, not
// an outage, so the fallback must not quietly phone the mirrors instead.
func TestCheckUpdatesEmptyBaseUrlStaysOffline(t *testing.T) {
	withHosts(t, "http://127.0.0.1:1")
	c := &Client{BaseURL: "", HTTP: &http.Client{Timeout: time.Second}}
	resp, err := c.CheckUpdates(context.Background(), "1.3.121", "alpha")
	if err != nil || resp != nil {
		t.Fatalf("empty BaseURL must stay a no-op, got resp=%+v err=%v", resp, err)
	}
}

// Both doors shut: the error must name the Worker's failure (the one that
// matters) and still show the fallback was attempted.
func TestCheckUpdatesBothDownReportsBoth(t *testing.T) {
	worker := manifestServer(t, 503, "text/plain", "down")
	withHosts(t, "http://127.0.0.1:1")

	c := &Client{BaseURL: worker.URL, HTTP: &http.Client{Timeout: 2 * time.Second}}
	_, err := c.CheckUpdates(context.Background(), "1.3.121", "alpha")
	if err == nil {
		t.Fatal("expected an error when both the Worker and the mirrors are down")
	}
	if got := err.Error(); !contains(got, "503") || !contains(got, "fallback also failed") {
		t.Fatalf("error should name the Worker failure and note the fallback: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

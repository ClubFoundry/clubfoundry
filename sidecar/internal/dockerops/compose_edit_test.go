package dockerops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const sampleCompose = `name: clubfoundry
services:
  clubfoundry:
    image: localhost:5000/clubfoundry/clubfoundry:latest
    container_name: clubfoundry
    network_mode: host
    volumes:
      - /mnt/data:/app/data
    env_file: /mnt/.env
    restart: unless-stopped

  clubfoundry-updater:
    image: localhost:5000/clubfoundry/updater:dev
    container_name: clubfoundry-updater
    network_mode: host
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /mnt/compose:/compose:ro
`

// writeCompose drops the sample compose file into a temp dir and returns a
// Config pointing at it. Tests read / rewrite via SetServiceImage and
// CurrentImageRef.
func writeCompose(t *testing.T, body string) Config {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("seed compose: %v", err)
	}
	return Config{ComposeDir: dir, MainService: "clubfoundry", UpdaterService: "clubfoundry-updater"}
}

func TestCurrentImageRef(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	ref, err := c.CurrentImageRef("clubfoundry")
	if err != nil {
		t.Fatalf("clubfoundry ref: %v", err)
	}
	if ref != "localhost:5000/clubfoundry/clubfoundry:latest" {
		t.Errorf("clubfoundry: got %q", ref)
	}
	updRef, err := c.CurrentImageRef("clubfoundry-updater")
	if err != nil {
		t.Fatalf("updater ref: %v", err)
	}
	if updRef != "localhost:5000/clubfoundry/updater:dev" {
		t.Errorf("updater: got %q", updRef)
	}
}

func TestCurrentImageRef_MissingService(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	_, err := c.CurrentImageRef("not-a-service")
	if err == nil {
		t.Fatal("expected error for missing service")
	}
}

func TestCurrentImageRef_QuotedValue(t *testing.T) {
	quoted := strings.Replace(
		sampleCompose,
		"image: localhost:5000/clubfoundry/clubfoundry:latest",
		`image: "localhost:5000/clubfoundry/clubfoundry:latest"`,
		1,
	)
	c := writeCompose(t, quoted)
	ref, err := c.CurrentImageRef("clubfoundry")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "localhost:5000/clubfoundry/clubfoundry:latest" {
		t.Fatalf("quoted ref = %q, want unquoted value", ref)
	}
}

// TestSetServiceImage_BareTag verifies that the most common stepped-update
// call shape — passing just a version tag like "1.0.30" — preserves the
// service's existing registry + repo and swaps only the tag portion.
func TestSetServiceImage_BareTag(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	if err := c.SetServiceImage("clubfoundry", "1.0.30"); err != nil {
		t.Fatalf("SetServiceImage: %v", err)
	}
	ref, _ := c.CurrentImageRef("clubfoundry")
	if ref != "localhost:5000/clubfoundry/clubfoundry:1.0.30" {
		t.Errorf("after bare-tag set: got %q", ref)
	}
	// Other service untouched.
	updRef, _ := c.CurrentImageRef("clubfoundry-updater")
	if updRef != "localhost:5000/clubfoundry/updater:dev" {
		t.Errorf("updater regressed: got %q", updRef)
	}
}

// TestSetServiceImage_FullRef passes a fully-qualified reference — should
// use it verbatim, not treat the tag portion as a bare tag to swap.
func TestSetServiceImage_FullRef(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	full := "ghcr.io/clubfoundry/clubfoundry:1.0.31"
	if err := c.SetServiceImage("clubfoundry", full); err != nil {
		t.Fatalf("SetServiceImage: %v", err)
	}
	ref, _ := c.CurrentImageRef("clubfoundry")
	if ref != full {
		t.Errorf("after full-ref set: got %q, want %q", ref, full)
	}
}

// TestSetServiceImage_RegistryPort — registry hosts like localhost:5000
// include a colon in the registry portion. The tag is the LAST colon's
// right side. Must not truncate at the registry's port colon.
func TestSetServiceImage_RegistryPort(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	if err := c.SetServiceImage("clubfoundry", "2.0.0"); err != nil {
		t.Fatalf("SetServiceImage: %v", err)
	}
	ref, _ := c.CurrentImageRef("clubfoundry")
	// Registry `localhost:5000` must be preserved; only the trailing tag swapped.
	if ref != "localhost:5000/clubfoundry/clubfoundry:2.0.0" {
		t.Errorf("registry port corrupted: %q", ref)
	}
}

// TestSetServiceImage_DigestForm — if the compose file pins an image by
// digest, we can't leave the digest in place when swapping tag (pull would
// fail on manifest mismatch). Strip digest, apply new tag.
func TestSetServiceImage_DigestForm(t *testing.T) {
	composeWithDigest := strings.Replace(
		sampleCompose,
		"localhost:5000/clubfoundry/clubfoundry:latest",
		"localhost:5000/clubfoundry/clubfoundry@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		1,
	)
	c := writeCompose(t, composeWithDigest)
	if err := c.SetServiceImage("clubfoundry", "1.0.30"); err != nil {
		t.Fatalf("SetServiceImage: %v", err)
	}
	ref, _ := c.CurrentImageRef("clubfoundry")
	if ref != "localhost:5000/clubfoundry/clubfoundry:1.0.30" {
		t.Errorf("digest not stripped: got %q", ref)
	}
}

// TestSetServiceImage_PreservesFormatting — the rewrite must not reshuffle
// other lines. Concretely: the `services:` block order, each service's
// child-key order, blank lines between services, comments — all preserved
// byte-for-byte except the single image: line we change.
func TestSetServiceImage_PreservesFormatting(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	if err := c.SetServiceImage("clubfoundry", "1.0.30"); err != nil {
		t.Fatalf("SetServiceImage: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(c.ComposeDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(data)
	// Only one line should have changed — count diff by splitting both.
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(sampleCompose, "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("line count drift: got %d want %d", len(gotLines), len(wantLines))
	}
	diffs := 0
	for i := range gotLines {
		if gotLines[i] != wantLines[i] {
			diffs++
		}
	}
	if diffs != 1 {
		t.Errorf("expected exactly 1 changed line, got %d", diffs)
	}
	// The preserved lines must include key markers — service header order
	// + blank separator + updater still untouched.
	for _, marker := range []string{
		"name: clubfoundry",
		"  clubfoundry:",
		"  clubfoundry-updater:",
		"    container_name: clubfoundry",
		"    image: localhost:5000/clubfoundry/updater:dev",
	} {
		if !strings.Contains(got, marker) {
			t.Errorf("missing marker after rewrite: %q", marker)
		}
	}
}

// TestSetServiceImage_MissingFile surfaces as an error, doesn't panic.
func TestSetServiceImage_MissingFile(t *testing.T) {
	dir := t.TempDir()
	c := Config{ComposeDir: dir}
	if err := c.SetServiceImage("clubfoundry", "1.0.30"); err == nil {
		t.Fatal("expected error for missing compose file")
	}
}

// TestSetServiceImage_BuildOnlyService — services with no `image:` line
// (build-only, rare in Mode B) must produce a clear error.
func TestSetServiceImage_BuildOnlyService(t *testing.T) {
	buildOnly := `services:
  builder:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: b
`
	c := writeCompose(t, buildOnly)
	if err := c.SetServiceImage("builder", "1.0.30"); err == nil {
		t.Fatal("expected error for build-only service")
	}
}

func TestResolveImageRef(t *testing.T) {
	cases := []struct {
		name    string
		current string
		newRef  string
		want    string
	}{
		{"bare tag onto registry-port", "localhost:5000/foo/bar:latest", "1.0.30", "localhost:5000/foo/bar:1.0.30"},
		{"bare tag onto plain registry", "ghcr.io/foo/bar:dev", "1.0.30", "ghcr.io/foo/bar:1.0.30"},
		{"full ref verbatim", "ghcr.io/foo/bar:dev", "ghcr.io/other/repo:v2", "ghcr.io/other/repo:v2"},
		{"bare tag onto repo-with-no-tag", "ghcr.io/foo/bar", "1.0.30", "ghcr.io/foo/bar:1.0.30"},
		{"bare tag strips digest", "repo@sha256:deadbeef", "1.0.30", "repo:1.0.30"},
		{"hub-style full ref", "mysql:8", "postgres:15", "postgres:15"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveImageRef(tc.current, tc.newRef)
			if got != tc.want {
				t.Errorf("resolveImageRef(%q, %q) = %q, want %q", tc.current, tc.newRef, got, tc.want)
			}
		})
	}
}

func TestLooksLikeFullRef(t *testing.T) {
	cases := []struct {
		in   string
		full bool
	}{
		{"1.0.30", false},
		{"1.0.30-rc.1", false},
		{"dev", false},
		{"latest", false},
		{"ghcr.io/foo/bar:1.0.30", true},
		{"localhost:5000/foo:1", true},
		{"nginx:1.25", true},
		{"mysql:8", true},
		{"", false},
	}
	for _, c := range cases {
		if got := looksLikeFullRef(c.in); got != c.full {
			t.Errorf("looksLikeFullRef(%q) = %v, want %v", c.in, got, c.full)
		}
	}
}

func TestStripTagOrDigest(t *testing.T) {
	cases := []struct{ in, want string }{
		{"localhost:5000/foo/bar:latest", "localhost:5000/foo/bar"},
		{"ghcr.io/foo/bar:1.0.30", "ghcr.io/foo/bar"},
		{"ghcr.io/foo/bar@sha256:deadbeef", "ghcr.io/foo/bar"},
		{"foo/bar:tag@sha256:deadbeef", "foo/bar:tag"}, // digest wins over tag suffix
		{"ghcr.io/foo/bar", "ghcr.io/foo/bar"},         // no tag, no digest
		{"localhost:5000/foo/bar", "localhost:5000/foo/bar"},
	}
	for _, c := range cases {
		if got := stripTagOrDigest(c.in); got != c.want {
			t.Errorf("stripTagOrDigest(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestComposeWriteFallbackHelpers(t *testing.T) {
	if !isBusyErr(syscall.EBUSY) {
		t.Fatal("direct EBUSY must be recognized")
	}
	wrapped := &os.LinkError{Op: "rename", Old: "old", New: "new", Err: syscall.EBUSY}
	if !isBusyErr(wrapped) {
		t.Fatal("wrapped EBUSY must be recognized")
	}
	if isBusyErr(errors.New("different failure")) {
		t.Fatal("non-EBUSY error must not be recognized")
	}

	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte("old-content"), 0o640); err != nil {
		t.Fatal(err)
	}
	want := []byte("services:\n  app:\n    image: repo:v2\n")
	if err := writeComposeInPlace(path, want); err != nil {
		t.Fatalf("writeComposeInPlace: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("in-place body = %q, want %q", got, want)
	}
}

// TestPreloadedImageRef verifies the reference used to detect a locally
// preloaded target image before downloading it again.
func TestPreloadedImageRef(t *testing.T) {
	c := writeCompose(t, sampleCompose)
	cases := []struct {
		service string
		tag     string
		want    string
		wantErr bool
	}{
		{"clubfoundry-updater", "v1.K", "localhost:5000/clubfoundry/updater:v1.K", false},
		{"clubfoundry", "1.1.93", "localhost:5000/clubfoundry/clubfoundry:1.1.93", false},
		// Full ref passed as `tag` is honored verbatim.
		{"clubfoundry-updater", "ghcr.io/foo/bar:v1.0", "ghcr.io/foo/bar:v1.0", false},
		// Empty inputs reject — caller falls through to download.
		{"", "v1.K", "", true},
		{"clubfoundry-updater", "", "", true},
		// Unknown service → CurrentImageRef errors → bubble up.
		{"non-existent", "v1.K", "", true},
	}
	for _, tc := range cases {
		got, err := c.PreloadedImageRef(tc.service, tc.tag)
		if tc.wantErr {
			if err == nil {
				t.Errorf("PreloadedImageRef(%q,%q) = %q, want error", tc.service, tc.tag, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("PreloadedImageRef(%q,%q) errored: %v", tc.service, tc.tag, err)
			continue
		}
		if got != tc.want {
			t.Errorf("PreloadedImageRef(%q,%q) = %q, want %q", tc.service, tc.tag, got, tc.want)
		}
	}
}

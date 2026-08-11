package dockerops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestDedupeNonEmpty(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, []string{}},
		{[]string{}, []string{}},
		{[]string{"a", "b"}, []string{"a", "b"}},
		{[]string{"a", "", "b"}, []string{"a", "b"}},
		{[]string{"a", "a", "b", "a"}, []string{"a", "b"}}, // dupes collapse, order preserved
		{[]string{"", "", ""}, []string{}},
	}
	for _, c := range cases {
		if got := dedupeNonEmpty(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("dedupeNonEmpty(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestDownloadToFile covers streaming, progress accounting, and SHA-256.
func TestDownloadToFile(t *testing.T) {
	payload := make([]byte, 1<<20) // 1 MiB of non-trivial bytes
	for i := range payload {
		payload[i] = byte(i * 31)
	}
	sum := sha256.Sum256(payload)
	wantSha := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	c := Config{}
	dir := t.TempDir()

	// Happy path: correct sha → file written byte-exact, bytes tracked.
	m := &mirrorDownload{url: srv.URL, path: filepath.Join(dir, "ok.tar.gz")}
	if err := c.downloadToFile(context.Background(), m, wantSha, nil, nil); err != nil {
		t.Fatalf("downloadToFile happy path: %v", err)
	}
	got, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("downloaded content mismatch: got %d bytes, want %d", len(got), len(payload))
	}
	if m.bytes.Load() != int64(len(payload)) {
		t.Errorf("m.bytes = %d, want %d", m.bytes.Load(), len(payload))
	}
	if m.total.Load() != int64(len(payload)) {
		t.Errorf("m.total = %d, want %d", m.total.Load(), len(payload))
	}

	// sha256 mismatch → hard error (refuse to load tampered tarball).
	mBad := &mirrorDownload{url: srv.URL, path: filepath.Join(dir, "bad.tar.gz")}
	if err := c.downloadToFile(context.Background(), mBad, "deadbeef", nil, nil); err == nil {
		t.Errorf("downloadToFile with wrong sha: want error, got nil")
	}

	// Empty sha → refused before any network call.
	mNoSha := &mirrorDownload{url: srv.URL, path: filepath.Join(dir, "nosha.tar.gz")}
	if err := c.downloadToFile(context.Background(), mNoSha, "", nil, nil); err == nil {
		t.Errorf("downloadToFile with empty sha: want error, got nil")
	}
}

func TestDownloadToFileHTTPContract(t *testing.T) {
	payload := []byte("chunked payload without a declared content length")
	sum := sha256.Sum256(payload)
	wantSha := hex.EncodeToString(sum[:])

	t.Run("unknown content length", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			_, _ = w.Write(payload)
		}))
		defer srv.Close()

		m := &mirrorDownload{url: srv.URL, path: filepath.Join(t.TempDir(), "image.tar.gz")}
		if err := (Config{}).downloadToFile(context.Background(), m, wantSha, nil, nil); err != nil {
			t.Fatalf("downloadToFile: %v", err)
		}
		if m.total.Load() != 0 {
			t.Fatalf("tracked total = %d, want 0 for unknown length", m.total.Load())
		}
		got, err := os.ReadFile(m.path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("downloaded body = %q, want %q", got, payload)
		}
	})

	t.Run("non-success status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		m := &mirrorDownload{url: srv.URL, path: filepath.Join(t.TempDir(), "image.tar.gz")}
		err := (Config{}).downloadToFile(context.Background(), m, wantSha, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
			t.Fatalf("error = %v, want HTTP 503", err)
		}
		if _, statErr := os.Stat(m.path); !os.IsNotExist(statErr) {
			t.Fatalf("destination created after rejected response: %v", statErr)
		}
	})
}

func TestLoadFromURLVerifiesBeforeDockerLoad(t *testing.T) {
	payload := []byte("single URL image artifact")
	sum := sha256.Sum256(payload)
	wantSha := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	newConfig := func(t *testing.T) (Config, string, string) {
		t.Helper()
		composeDir := t.TempDir()
		dataDir := filepath.Join(composeDir, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatal(err)
		}
		argsPath := filepath.Join(t.TempDir(), "docker-args.txt")
		t.Setenv("FAKE_DOCKER_ARGS_FILE", argsPath)
		t.Setenv("FAKE_DOCKER_STDOUT", "Loaded image: repo/app:v1")
		return Config{DockerBin: writeFakeDocker(t), ComposeDir: composeDir}, dataDir, argsPath
	}
	assertTempClean := func(t *testing.T, dataDir string) {
		t.Helper()
		entries, err := os.ReadDir(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("temporary artifact was not removed: %v", entries)
		}
	}

	t.Run("checksum mismatch never invokes docker", func(t *testing.T) {
		cfg, dataDir, argsPath := newConfig(t)
		err := cfg.loadFromURL(context.Background(), "", "v1", PullOpts{
			URL:    srv.URL,
			Sha256: "deadbeef",
		})
		if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("error = %v, want sha256 mismatch", err)
		}
		if data, readErr := os.ReadFile(argsPath); readErr == nil {
			t.Fatalf("docker was invoked before verification: %q", data)
		} else if !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
		assertTempClean(t, dataDir)
	})

	t.Run("valid checksum loads and removes temporary artifact", func(t *testing.T) {
		cfg, dataDir, argsPath := newConfig(t)
		if err := cfg.loadFromURL(context.Background(), "", "v1", PullOpts{
			URL:    srv.URL,
			Sha256: wantSha,
		}); err != nil {
			t.Fatalf("loadFromURL: %v", err)
		}
		args, err := os.ReadFile(argsPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(args)) != "load" {
			t.Fatalf("docker arguments = %q, want load", args)
		}
		assertTempClean(t, dataDir)
	})
}

func TestFirstMirrorErrorContract(t *testing.T) {
	want := errors.New("first mirror failed")
	first := &mirrorDownload{done: make(chan error, 1)}
	first.failed.Store(true)
	first.done <- want
	second := &mirrorDownload{done: make(chan error, 1)}
	second.failed.Store(true)
	second.done <- errors.New("second mirror failed")

	if got := firstMirrorErr([]*mirrorDownload{first, second}); !errors.Is(got, want) {
		t.Fatalf("firstMirrorErr = %v, want %v", got, want)
	}
	if got := firstMirrorErr([]*mirrorDownload{{}}); got == nil || got.Error() != "no successful mirror" {
		t.Fatalf("empty failure set = %v, want no successful mirror", got)
	}
}

func TestParseLoadedImageContract(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{"Loaded image: repo/app:v1\n", "repo/app:v1"},
		{"noise\nLoaded image: first:v1\nLoaded image: second:v2\n", "first:v1"},
		{"Loaded image ID: sha256:deadbeef\n", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := parseLoadedImage(tc.output); got != tc.want {
			t.Errorf("parseLoadedImage(%q) = %q, want %q", tc.output, got, tc.want)
		}
	}
}

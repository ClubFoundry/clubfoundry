package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

func TestRunSkipsExistingCompose(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := Deps{Docker: dockerops.Config{ComposeDir: dir}}
	if err := d.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error for existing compose: %v", err)
	}
}

func TestMainHealthyRequiresHTTP200(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		want bool
	}{
		{name: "healthy", code: http.StatusOK, want: true},
		{name: "unhealthy", code: http.StatusServiceUnavailable, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer server.Close()

			if got := mainHealthy(context.Background(), server.URL, time.Second); got != tc.want {
				t.Fatalf("mainHealthy() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWaitMainHealthyReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitMainHealthy(ctx, "http://127.0.0.1:1/health", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitMainHealthy() error = %v, want context.Canceled", err)
	}
}

func TestHostPaths(t *testing.T) {
	t.Setenv("CLUBFOUNDRY_HOST_DATA_DIR", "")
	t.Setenv("CLUBFOUNDRY_HOST_COMPOSE_FILE", "")
	if got := hostDataDir(); got != "/opt/clubfoundry/data" {
		t.Fatalf("hostDataDir() = %q", got)
	}
	if got := hostComposePath("ignored"); got != "/opt/clubfoundry/docker-compose.yml" {
		t.Fatalf("hostComposePath() = %q", got)
	}

	t.Setenv("CLUBFOUNDRY_HOST_DATA_DIR", "/srv/clubfoundry/data")
	t.Setenv("CLUBFOUNDRY_HOST_COMPOSE_FILE", "/srv/clubfoundry/compose.yml")
	if got := hostDataDir(); got != "/srv/clubfoundry/data" {
		t.Fatalf("hostDataDir() override = %q", got)
	}
	if got := hostComposePath("ignored"); got != "/srv/clubfoundry/compose.yml" {
		t.Fatalf("hostComposePath() override = %q", got)
	}
}

func TestOrDev(t *testing.T) {
	for input, want := range map[string]string{
		"":        "latest",
		"dev":     "latest",
		"DEV":     "latest",
		"v3.AH":   "v3.AH",
		"1.3.143": "1.3.143",
	} {
		if got := orDev(input); got != want {
			t.Fatalf("orDev(%q) = %q, want %q", input, got, want)
		}
	}
}

package dockerops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractTag(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ghcr.io/clubfoundry/clubfoundry:1.0.30", "1.0.30"},
		{"clubfoundry:latest", "latest"},
		{"clubfoundry:1.0.30@sha256:deadbeef", "1.0.30"},
		{"ghcr.io:443/foo/bar", ""}, // registry port, no tag
		{"foo/bar:tag@sha256:abc", "tag"},
	}
	for _, c := range cases {
		if got := extractTag(c.in); got != c.want {
			t.Errorf("extractTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParsePSLineForm(t *testing.T) {
	raw := `{"Service":"clubfoundry","Image":"ghcr.io/clubfoundry/clubfoundry:1.0.30","State":"running"}
{"Service":"clubfoundry-updater","Image":"ghcr.io/clubfoundry/updater:latest","State":"running"}`
	out, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 services, got %d", len(out))
	}
	if out[0].Tag != "1.0.30" || out[1].Tag != "latest" {
		t.Errorf("tag parse: %+v", out)
	}
}

func TestParsePSArrayForm(t *testing.T) {
	raw := `[
		{"Service":"clubfoundry","Image":"ghcr.io/clubfoundry/clubfoundry:1.0.30","State":"running"},
		{"Service":"clubfoundry-updater","Image":"ghcr.io/clubfoundry/updater:latest","State":"exited"}
	]`

	out, err := parsePS(raw)
	if err != nil {
		t.Fatalf("parsePS err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 services, got %d", len(out))
	}
	if out[0].Tag != "1.0.30" || out[1].State != "exited" {
		t.Fatalf("unexpected parsed services: %+v", out)
	}
}

func TestContainsContainerNameConflictContract(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{
			name: "daemon conflict with slash",
			out:  `Conflict. The container name "/clubfoundry" is already in use by container "abc".`,
			want: true,
		},
		{
			name: "daemon conflict without slash",
			out:  `container name "clubfoundry" is already in use`,
			want: true,
		},
		{
			name: "different container",
			out:  `container name "/clubfoundry-updater" is already in use`,
		},
		{
			name: "same name unrelated error",
			out:  `cannot stop container "/clubfoundry"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsContainerNameConflict([]byte(tt.out), "clubfoundry"); got != tt.want {
				t.Fatalf("containsContainerNameConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClubFoundryImageOwnershipContract(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{ref: "clubfoundry:1.3.143", want: true},
		{ref: "clubfoundry-updater:v3.AL", want: true},
		{ref: "ghcr.io/clubfoundry/clubfoundry:1.3.143", want: true},
		{ref: "registry.example/namespace/clubfoundry-updater:v3.AL", want: true},
		{ref: "clubfoundry", want: false},
		{ref: "postgres:16", want: false},
		{ref: "ghcr.io/clubfoundry/not-clubfoundry:1.0", want: false},
		{ref: "", want: false},
	}
	for _, tt := range tests {
		if got := isClubFoundryImage(tt.ref); got != tt.want {
			t.Errorf("isClubFoundryImage(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestForceRemoveContainerChecksOwnership(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	t.Setenv("FAKE_DOCKER_ARGS_FILE", argsPath)
	cfg := Config{DockerBin: writeFakeDocker(t), ComposeDir: dir, DefaultTimeout: time.Second}

	readArgs := func(t *testing.T) []string {
		t.Helper()
		body, err := os.ReadFile(argsPath)
		if err != nil {
			t.Fatal(err)
		}
		return strings.FieldsFunc(strings.TrimSpace(string(body)), func(r rune) bool { return r == '\n' || r == '\r' })
	}
	resetArgs := func(t *testing.T) {
		t.Helper()
		if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("foreign image fails closed", func(t *testing.T) {
		resetArgs(t)
		t.Setenv("FAKE_DOCKER_STDOUT", "postgres:16")
		if err := cfg.forceRemoveContainer(context.Background(), "clubfoundry"); err == nil || !strings.Contains(err.Error(), "is not ClubFoundry") {
			t.Fatalf("forceRemoveContainer() error = %v", err)
		}
		args := readArgs(t)
		if len(args) != 1 || !strings.Contains(args[0], "inspect clubfoundry") {
			t.Fatalf("foreign container commands = %q, want inspect only", args)
		}
	})

	t.Run("inspect failure fails closed", func(t *testing.T) {
		resetArgs(t)
		t.Setenv("FAKE_DOCKER_STDOUT", "inspect failed")
		t.Setenv("FAKE_DOCKER_EXIT", "1")
		if err := cfg.forceRemoveContainer(context.Background(), "clubfoundry"); err == nil || !strings.Contains(err.Error(), "ownership check failed") {
			t.Fatalf("forceRemoveContainer() error = %v", err)
		}
		args := readArgs(t)
		if len(args) != 1 || !strings.Contains(args[0], "inspect clubfoundry") {
			t.Fatalf("inspect failure commands = %q, want inspect only", args)
		}
	})

	t.Run("owned image can be removed", func(t *testing.T) {
		resetArgs(t)
		t.Setenv("FAKE_DOCKER_STDOUT", "ghcr.io/clubfoundry/clubfoundry:1.3.143")
		t.Setenv("FAKE_DOCKER_EXIT", "")
		if err := cfg.forceRemoveContainer(context.Background(), "clubfoundry"); err != nil {
			t.Fatalf("forceRemoveContainer() error = %v", err)
		}
		args := readArgs(t)
		if len(args) != 2 || !strings.Contains(args[0], "inspect clubfoundry") || !strings.Contains(args[1], "rm -f clubfoundry") {
			t.Fatalf("owned container commands = %q, want inspect then rm", args)
		}
	})
}

package health

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMainPortFromEnvFile(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"plain", "CLM_PORT=3080\n", "3080"},
		{"default-3000", "CLM_PORT=3000\n", "3000"},
		{"quoted", "CLM_PORT=\"8723\"\n", "8723"},
		{"single-quoted", "CLM_PORT='9444'\n", "9444"},
		{"export-prefix", "export CLM_PORT=3400\n", "3400"},
		{"leading-ws", "   CLM_PORT=3080  \n", "3080"},
		{"trailing-comment", "CLM_PORT=3080 # web ui\n", "3080"},
		{"crlf", "CLM_PORT=3080\r\n", "3080"},
		{"among-others", "CLM_TRUENAS_HOST=\nCLM_PORT=8910\nCLM_DATA_DIR=/app/data\n", "8910"},
		{"missing-key", "CLM_TRUENAS_HOST=\nCLM_DATA_DIR=/app/data\n", ""},
		{"blank-value", "CLM_PORT=\n", ""},
		{"non-numeric", "CLM_PORT=abc\n", ""},
		{"commented-out", "# CLM_PORT=3000\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, ".env")
			if err := os.WriteFile(p, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := mainPortFromEnvFile(p); got != c.want {
				t.Fatalf("mainPortFromEnvFile(%q) = %q, want %q", c.body, got, c.want)
			}
		})
	}
}

func TestMainPortFromEnvFile_AbsentFile(t *testing.T) {
	if got := mainPortFromEnvFile(filepath.Join(t.TempDir(), "nope.env")); got != "" {
		t.Fatalf("absent file: got %q, want \"\"", got)
	}
}

func TestResolveMainHealthURL(t *testing.T) {
	// 1. Explicit env override always wins.
	t.Setenv("CLUBFOUNDRY_HEALTH_URL", "http://127.0.0.1:5555/health")
	if got := ResolveMainHealthURL(); got != "http://127.0.0.1:5555/health" {
		t.Fatalf("override: got %q", got)
	}

	// 2. No override → read CLM_PORT from <DATA_DIR>/.env.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("CLM_PORT=8723\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLUBFOUNDRY_HEALTH_URL", "")
	t.Setenv("CLUBFOUNDRY_DATA_DIR", dir)
	if got := ResolveMainHealthURL(); got != "http://127.0.0.1:8723/health" {
		t.Fatalf("from .env: got %q, want http://127.0.0.1:8723/health", got)
	}

	// 3. No override + no CLM_PORT → conventional 3000 default.
	empty := t.TempDir()
	t.Setenv("CLUBFOUNDRY_DATA_DIR", empty)
	if got := ResolveMainHealthURL(); got != "http://127.0.0.1:3000/health" {
		t.Fatalf("default: got %q, want http://127.0.0.1:3000/health", got)
	}
}

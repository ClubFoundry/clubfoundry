// Tests for security-critical docker-compose.yml secret redaction. False
// negatives can leak credentials to a
// shared bundle file → over-test what's redacted, then sample a real
// fixture to confirm intentional passthroughs (image refs, ports, etc.)
// stay readable.
package handlers

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticBundleHTTPContract(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "sidecar-state")
	sentinelDir := filepath.Join(stateDir, "recreate-status")
	logEntryDir := filepath.Join(dataDir, "update-logs", "upd-1")
	failureEntryDir := filepath.Join(dataDir, "update-failures", "fail-1")
	for _, dir := range []string{sentinelDir, logEntryDir, failureEntryDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(stateDir, "main.json"):                  `{"phase":"idle"}`,
		filepath.Join(stateDir, "self.json"):                  `{"phase":"checking"}`,
		filepath.Join(sentinelDir, "trampoline.json"):         `{"exit_code":0}`,
		filepath.Join(sentinelDir, "ignored.txt"):             "ignored",
		filepath.Join(logEntryDir, "update.log"):              "updated",
		filepath.Join(failureEntryDir, "failure-bundle.json"): `{"outcome":"rollback"}`,
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	composeDir := t.TempDir()
	compose := "services:\n  app:\n    image: clubfoundry:1.2.3\n    environment:\n      DB_PASSWORD: top-secret\n"
	if err := os.WriteFile(filepath.Join(composeDir, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLUBFOUNDRY_COMPOSE_DIR", composeDir)

	rr := httptest.NewRecorder()
	handleDiagnosticBundle(dataDir, "v3.TEST").ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/diagnostic-bundle", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}

	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	entries := make(map[string]string, len(zr.File))
	for _, file := range zr.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(body)
	}
	for _, name := range []string{
		"INFO.txt",
		"sidecar-state/main.json",
		"sidecar-state/self.json",
		"sidecar-state/recreate-status/trampoline.json",
		"update-logs/upd-1/update.log",
		"update-failures/fail-1/failure-bundle.json",
		"docker-compose.yml",
	} {
		if _, ok := entries[name]; !ok {
			t.Errorf("bundle missing %q", name)
		}
	}
	if _, ok := entries["sidecar-state/recreate-status/ignored.txt"]; ok {
		t.Fatal("non-JSON sentinel was included")
	}
	if strings.Contains(entries["docker-compose.yml"], "top-secret") || !strings.Contains(entries["docker-compose.yml"], "<REDACTED>") {
		t.Fatalf("compose secret was not redacted: %q", entries["docker-compose.yml"])
	}
}

func TestRedactSecrets_PasswordKeyValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // expected substring after redaction
	}{
		{"yaml password", `      DB_PASSWORD: supersecret`, `<REDACTED>`},
		{"yaml lowercase password", `      password: hunter2`, `<REDACTED>`},
		{"env-list dash style", `      - DB_PASSWORD=supersecret`, `<REDACTED>`},
		{"key=value env_file format", `DB_PASSWORD=supersecret`, `<REDACTED>`},
		{"jwt secret", `JWT_SECRET=blah-blah`, `<REDACTED>`},
		{"api key", `BREVO_API_KEY=xxxxx`, `<REDACTED>`},
		{"truenas api key", `CLM_TRUENAS_API_KEY=1-tkn-foobar`, `<REDACTED>`},
		{"private key", `SSH_PRIVATE: PEM-INLINE`, `<REDACTED>`},
		{"auth token", `AUTH_TOKEN: jwt.body.sig`, `<REDACTED>`},
		{"credential", `MY_CREDENTIAL=foo`, `<REDACTED>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(redactSecrets([]byte(tc.in)))
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in output, got %q (input was %q)", tc.want, out, tc.in)
			}
			if strings.Contains(out, "supersecret") || strings.Contains(out, "hunter2") ||
				strings.Contains(out, "blah-blah") || strings.Contains(out, "xxxxx") ||
				strings.Contains(out, "1-tkn-foobar") || strings.Contains(out, "PEM-INLINE") ||
				strings.Contains(out, "jwt.body.sig") || strings.Contains(out, "foo") {
				// "foo" check is conservative — last case redacts MY_CREDENTIAL=foo,
				// so "foo" must not appear in output.
				t.Errorf("secret value leaked through: %q", out)
			}
		})
	}
}

func TestRedactSecrets_NonSecretPassthrough(t *testing.T) {
	// Lines that should NOT be redacted (image refs, ports, names).
	// Critical because over-redaction breaks the bundle's diagnostic value
	// (operator can't see what version was running, what port mapped, etc.).
	cases := []string{
		`    image: clubfoundry:1.1.89`,
		`      - "3000:3000"`,
		`    container_name: clubfoundry`,
		`    restart: unless-stopped`,
		`    network_mode: host`,
		`  clubfoundry-updater:`,
		`    volumes:`,
		`      - /var/run/docker.sock:/var/run/docker.sock`,
		`version: "3.8"`,
	}
	for _, line := range cases {
		out := string(redactSecrets([]byte(line)))
		if strings.Contains(out, "<REDACTED>") {
			t.Errorf("non-secret line was redacted: input=%q output=%q", line, out)
		}
	}
}

func TestRedactSecrets_PreservesIndent(t *testing.T) {
	in := "      DB_PASSWORD: secretvalue"
	out := string(redactSecrets([]byte(in)))
	// Leading indent must be preserved so yaml structure stays parseable.
	if !strings.HasPrefix(out, "      ") {
		t.Errorf("leading indent stripped: %q", out)
	}
	// Key + separator must remain.
	if !strings.Contains(out, "DB_PASSWORD: ") {
		t.Errorf("key+separator stripped: %q", out)
	}
}

func TestRedactSecrets_RealComposeFixture(t *testing.T) {
	// Realistic ClubFoundry compose snippet with mixed secret + non-secret
	// lines. End-to-end check that the regex behaves as expected on the
	// shape we'll see in production.
	fixture := `version: "3.8"
services:
  clubfoundry:
    image: clubfoundry:1.1.89
    container_name: clubfoundry
    restart: unless-stopped
    network_mode: host
    environment:
      - CLM_TRUENAS_HOST=https://192.0.2.10
      - CLM_TRUENAS_API_KEY=1-tnt-supersecret-token
      - JWT_SECRET=very-secret-jwt-signing-key
      - DB_PASSWORD=db-password-here
      - CLM_LOG_LEVEL=info
    volumes:
      - clubfoundry-data:/app/data
  clubfoundry-updater:
    image: clubfoundry-updater:v1.Y
    network_mode: host
    environment:
      - BREVO_API_KEY=xkeysib-leaks-here
      - CLUBFOUNDRY_DATA_DIR=/app/data
volumes:
  clubfoundry-data:
`
	out := string(redactSecrets([]byte(fixture)))
	mustContain := []string{
		"clubfoundry:1.1.89", // image — passthrough
		"network_mode: host", // non-secret passthrough
		"CLM_LOG_LEVEL=info", // log-level — passthrough
		"CLM_TRUENAS_HOST=",  // host URL — passthrough
		"<REDACTED>",         // at least one redaction happened
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in redacted output. full output:\n%s", want, out)
		}
	}
	mustNotContain := []string{
		"1-tnt-supersecret-token",     // API key
		"very-secret-jwt-signing-key", // JWT secret
		"db-password-here",            // DB password
		"xkeysib-leaks-here",          // Brevo key
	}
	for _, leak := range mustNotContain {
		if strings.Contains(out, leak) {
			t.Errorf("secret value leaked: %q. full output:\n%s", leak, out)
		}
	}
	// Count how many redactions happened — should be exactly the 4 secret
	// lines, no more no less. Catches false positives that would over-redact
	// legitimate config.
	got := strings.Count(out, "<REDACTED>")
	if got != 4 {
		t.Errorf("expected 4 <REDACTED> markers (one per secret), got %d. full output:\n%s", got, out)
	}
}

func TestRedactSecrets_IdempotentOnRedactedInput(t *testing.T) {
	// Re-running redactSecrets on already-redacted text shouldn't introduce
	// new redactions or break structure. Idempotent guard against accidental
	// double-redaction in some pipeline.
	in := []byte("DB_PASSWORD=<REDACTED>\nimage: foo:1.0")
	out1 := redactSecrets(in)
	out2 := redactSecrets(out1)
	if string(out1) != string(out2) {
		t.Errorf("redactSecrets not idempotent: first=%q second=%q", string(out1), string(out2))
	}
}

func TestRedactSecrets_EmptyAndCommentLines(t *testing.T) {
	// Empty + comment-only lines should pass through untouched.
	cases := []string{
		"",
		"# just a comment",
		"   # indented comment",
		"---",
	}
	for _, line := range cases {
		out := string(redactSecrets([]byte(line)))
		if out != line {
			t.Errorf("expected %q unchanged, got %q", line, out)
		}
	}
}

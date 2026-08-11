package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tokenValue(t *Token) string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.value
}

// TestInit_GeneratesTokenOnFirstBoot covers the happy clean-install path:
// data dir exists, no token file, Init creates one with mode 0644 + 64 hex.
func TestInit_GeneratesTokenOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if tokenValue(tok) == "" {
		t.Fatal("Init returned empty token in auth-on mode")
	}
	if !isValidTokenFormat(tokenValue(tok)) {
		t.Fatalf("generated token has wrong format: %q", tokenValue(tok))
	}

	path := filepath.Join(dir, tokenStateDir, tokenFileName)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("token file missing: %v", err)
	}
	// File-mode bits: only check user perms on Windows since 0644 doesn't
	// translate cleanly. On unix we expect exactly 0644 (group/world read
	// is required because backend container runs as `clm` while sidecar
	// runs as root — see auth.go fileMode comment).
	if runtime.GOOS != "windows" {
		if st.Mode().Perm() != 0o644 {
			t.Fatalf("token file mode = %o, want 0644", st.Mode().Perm())
		}
	}
}

// TestInit_ReusesExistingToken covers sidecar restarts: token persists,
// Init reads it back as-is.
func TestInit_ReusesExistingToken(t *testing.T) {
	dir := t.TempDir()
	tok1, err := Init(dir)
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}
	tok2, err := Init(dir)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if tokenValue(tok1) != tokenValue(tok2) {
		t.Fatalf("token regenerated across restarts: %q → %q", tokenValue(tok1), tokenValue(tok2))
	}
}

// TestInit_DirIsBackendTraversable verifies that the root-owned token
// directory remains traversable by the unprivileged backend container.
// The shared-volume contract requires directory mode 0755 and file mode 0644.
func TestInit_DirIsBackendTraversable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix dir-mode bits not meaningful on windows")
	}
	dir := t.TempDir()
	if _, err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tokDir := filepath.Join(dir, tokenStateDir)
	st, err := os.Stat(tokDir)
	if err != nil {
		t.Fatalf("stat %s: %v", tokDir, err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("token dir mode = %o, want 0755 for non-owner backend traversal", st.Mode().Perm())
	}
}

// TestInit_HealsLegacyDirMode0700 verifies that an existing private token
// directory is made backend-traversable without operator cleanup.
func TestInit_HealsLegacyDirMode0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix dir-mode bits not meaningful on windows")
	}
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, tokenStateDir)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("seed legacy dir: %v", err)
	}
	if err := os.Chmod(legacyDir, 0o700); err != nil {
		// MkdirAll may have applied umask; force exactly 0700.
		t.Fatalf("seed legacy chmod: %v", err)
	}
	if _, err := Init(dir); err != nil {
		t.Fatalf("Init on legacy dir: %v", err)
	}
	st, err := os.Stat(legacyDir)
	if err != nil {
		t.Fatalf("stat legacy dir: %v", err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("legacy dir mode after Init = %o, want 0755 (heal must run on every boot)", st.Mode().Perm())
	}
}

// TestInit_RegeneratesMalformed covers corrupt token-file scenario:
// truncated / wrong-charset / wrong-length file is replaced.
func TestInit_RegeneratesMalformed(t *testing.T) {
	dir := t.TempDir()
	badDir := filepath.Join(dir, tokenStateDir)
	if err := os.MkdirAll(badDir, dirMode); err != nil {
		t.Fatal(err)
	}
	// Half-length token (32 hex chars instead of 64).
	bad := "abcdef0123456789abcdef0123456789"
	path := filepath.Join(badDir, tokenFileName)
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if tokenValue(tok) == bad {
		t.Fatal("malformed token was reused — should have regenerated")
	}
	if !isValidTokenFormat(tokenValue(tok)) {
		t.Fatalf("regenerated token is malformed: %q", tokenValue(tok))
	}
}

// TestInit_NoAuthMode covers the empty-data-dir case used by tests +
// legacy install paths.
func TestInit_NoAuthMode(t *testing.T) {
	tok, err := Init("")
	if err != nil {
		t.Fatalf("Init(\"\"): %v", err)
	}
	if tokenValue(tok) != "" {
		t.Fatalf("no-auth mode returned non-empty token: %q", tokenValue(tok))
	}
	if !tok.Check("") {
		t.Fatal("no-auth Check(\"\") = false, want true")
	}
	if !tok.Check("anything-bearer-here") {
		t.Fatal("no-auth Check(any) = false, want true")
	}
}

func TestValidTokenFormatIsLowercaseHexOnly(t *testing.T) {
	valid := strings.Repeat("ab01", 16)
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{value: valid, want: true},
		{value: strings.ToUpper(valid), want: false},
		{value: valid[:63], want: false},
		{value: valid[:63] + "g", want: false},
	} {
		if got := isValidTokenFormat(tc.value); got != tc.want {
			t.Fatalf("isValidTokenFormat(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestNilTokenAllowsLegacyNoAuth(t *testing.T) {
	var tok *Token
	if !tok.Check("anything") {
		t.Fatal("nil Token rejected legacy no-auth request")
	}
}

// TestCheck_ConstantTimeMatch covers the auth-on happy path.
func TestCheck_ConstantTimeMatch(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !tok.Check(tokenValue(tok)) {
		t.Fatal("Check(value) = false, want true")
	}
	if tok.Check("") {
		t.Fatal("Check(\"\") = true, want false")
	}
	if tok.Check("not-the-token") {
		t.Fatal("Check(wrong) = true, want false")
	}
	// Differs only in last byte — constant-time compare must still reject.
	right := tokenValue(tok)
	wrong := right[:len(right)-1] + "0"
	if right == wrong {
		// Vanishingly unlikely (would need final hex char to already be 0)
		// but guard so the test stays meaningful.
		wrong = right[:len(right)-1] + "1"
	}
	if tok.Check(wrong) {
		t.Fatal("Check(off-by-one) = true, want false")
	}
}

// TestMiddleware_HealthAnon — /health passes without a header even in auth-on mode.
func TestMiddleware_HealthAnon(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(tok.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", resp.StatusCode)
	}
}

// TestMiddleware_GatedNoHeader — non-anon path with no Authorization header → 401.
func TestMiddleware_GatedNoHeader(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(tok.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/update")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/update no-header status = %d, want 401", resp.StatusCode)
	}
	if want, got := `Bearer realm="clubfoundry-sidecar"`, resp.Header.Get("WWW-Authenticate"); got != want {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, want)
	}
}

// TestMiddleware_GatedCorrectBearer — non-anon path with matching token → 200.
func TestMiddleware_GatedCorrectBearer(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(tok.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenValue(tok))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestMiddleware_GatedWrongBearer — non-matching token returns 401.
func TestMiddleware_GatedWrongBearer(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(tok.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer not-the-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestMiddleware_GatedNonBearerScheme — Basic / Digest / etc. → 401.
func TestMiddleware_GatedNonBearerScheme(t *testing.T) {
	dir := t.TempDir()
	tok, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(tok.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Basic auth WITH the right token would still be wrong scheme.
	req.Header.Set("Authorization", "Basic "+tokenValue(tok))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Basic-scheme status = %d, want 401", resp.StatusCode)
	}
}

// TestMiddleware_NoAuthMode — empty-token Token serves everything.
func TestMiddleware_NoAuthMode(t *testing.T) {
	tok, err := Init("")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(tok.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/update")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("no-auth status = %d, want 200", resp.StatusCode)
	}
}

// TestExtractBearer — parser corner cases.
func TestExtractBearer(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"Bearer abc", "abc"},
		{"Bearer  abc  ", "abc"}, // leading+trailing spaces trimmed
		{"bearer abc", ""},       // strict case
		{"Basic abc", ""},
		{"Bearer", ""},
		{"abc", ""},
	}
	for _, c := range cases {
		got := extractBearer(c.in)
		if got != c.want {
			t.Errorf("extractBearer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRedact — debug helper never leaks the full token.
func TestRedact(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "(empty)"},
		{"abcd", "(short)"},
		{"abcdefgh", "abcd..."},
		{"abcdefghijk", "abcd..."},
	}
	for _, c := range cases {
		got := redact(c.in)
		if got != c.want {
			t.Errorf("redact(%q) = %q, want %q", c.in, got, c.want)
		}
		if c.in != "" && len(c.in) >= 8 && strings.Contains(got, c.in[4:]) {
			t.Errorf("redact(%q) leaked tail: %q", c.in, got)
		}
	}
}

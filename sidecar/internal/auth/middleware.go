package auth

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

// Only the container health endpoint is intentionally anonymous.
var anonPaths = map[string]struct{}{
	"/health": {},
}

// Check compares a supplied bearer with the stored token in constant time.
func (t *Token) Check(bearer string) bool {
	if t == nil {
		return true
	}
	t.mu.RLock()
	stored := t.value
	t.mu.RUnlock()
	if stored == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(bearer), []byte(stored)) == 1
}

// Middleware allows anonymous health probes and authenticates every other path.
func (t *Token) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, anon := anonPaths[r.URL.Path]; anon {
			next.ServeHTTP(w, r)
			return
		}
		bearer := extractBearer(r.Header.Get("Authorization"))
		if !t.Check(bearer) {
			log.Printf("auth: 401 from %s on %s (bearer=%s)", r.RemoteAddr, r.URL.Path, redact(bearer))
			w.Header().Set("WWW-Authenticate", `Bearer realm="clubfoundry-sidecar"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearer(h string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// redact returns a log-safe bearer prefix without exposing the secret.
func redact(s string) string {
	if s == "" {
		return "(empty)"
	}
	if len(s) < 8 {
		return "(short)"
	}
	return s[:4] + "..."
}

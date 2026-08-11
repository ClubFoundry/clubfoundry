// Package cloud provides update discovery and resilient owned-domain access.
package cloud

import (
	"strings"
	"time"
)

// Pinned public IPv4 resolvers are queried in parallel.
var pinnedResolvers = []string{
	"1.1.1.1:53",
	"1.0.0.1:53",
	"9.9.9.9:53",
	"8.8.8.8:53",
}

// Only owned domains bypass the system resolver by default.
var ownedDomains = []string{
	".clubfoundry.net",
	".workers.dev",
}

const (
	resolverDialTimeout = 3 * time.Second
	resolverReadTimeout = 5 * time.Second
	dialerTimeout       = 30 * time.Second
	cacheTTL            = 1 * time.Hour
	forceIPv4Network    = "tcp4"
)

// IsOwnedDomain reports whether host is handled by the pinned resolver chain.
func IsOwnedDomain(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	for _, suffix := range ownedDomains {
		bare := strings.TrimPrefix(suffix, ".")
		if h == bare || strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

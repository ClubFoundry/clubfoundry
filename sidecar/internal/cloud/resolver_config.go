package cloud

import (
	"net"
	"sync"
	"time"
)

// DNSChain is a thread-safe IPv4 resolver and dialer with a TTL cache.
type DNSChain struct {
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	ips    []net.IP
	expiry time.Time
}

// NewDNSChain returns an independent resolver with an empty TTL cache.
func NewDNSChain() *DNSChain {
	return &DNSChain{cache: make(map[string]cacheEntry)}
}

var (
	sharedChain     *DNSChain
	sharedChainOnce sync.Once
)

// SharedChain returns the process-wide resolver and its shared cache.
func SharedChain() *DNSChain {
	sharedChainOnce.Do(func() { sharedChain = NewDNSChain() })
	return sharedChain
}

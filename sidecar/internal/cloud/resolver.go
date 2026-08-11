package cloud

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Lookup resolves IPv4 addresses through the owned-domain fallback policy.
func (d *DNSChain) Lookup(ctx context.Context, host string) ([]net.IP, error) {
	d.mu.RLock()
	if e, ok := d.cache[host]; ok && time.Now().Before(e.expiry) {
		d.mu.RUnlock()
		return e.ips, nil
	}
	d.mu.RUnlock()

	if !IsOwnedDomain(host) {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		d.store(host, ips)
		return ips, nil
	}

	ips, err := d.raceResolve(ctx, host)
	if err != nil {
		sysIPs, sysErr := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if sysErr != nil {
			return nil, fmt.Errorf("dns chain: %w (system fallback: %v)", err, sysErr)
		}
		d.store(host, sysIPs)
		return sysIPs, nil
	}
	d.store(host, ips)
	return ips, nil
}

func (d *DNSChain) store(host string, ips []net.IP) {
	d.mu.Lock()
	d.cache[host] = cacheEntry{ips: ips, expiry: time.Now().Add(cacheTTL)}
	d.mu.Unlock()
}

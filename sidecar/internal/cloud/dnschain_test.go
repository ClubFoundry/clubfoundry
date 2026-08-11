package cloud

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestIsOwnedDomain(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"clubfoundry.net", true},
		{"downloads.clubfoundry.net", true},
		{"api.downloads.clubfoundry.net", true},
		{"clubfoundry-cloud.mamont718.workers.dev", true},
		{"workers.dev", true},
		{"google.com", false},
		{"clubfoundry.example.com", false},
		{"notclubfoundry.net", false},
		{"clubfoundry.net.", true}, // trailing dot
		{"CLUBFOUNDRY.NET", true},  // case
	}
	for _, tc := range cases {
		if got := IsOwnedDomain(tc.host); got != tc.want {
			t.Errorf("IsOwnedDomain(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestLookupCacheHit(t *testing.T) {
	d := NewDNSChain()
	ip := net.ParseIP("203.0.113.7").To4()
	d.store("preseed.example.com", []net.IP{ip})

	got, err := d.Lookup(context.Background(), "preseed.example.com")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 || !got[0].Equal(ip) {
		t.Errorf("Lookup() = %v, want [%v]", got, ip)
	}
}

func TestLookupCacheExpiry(t *testing.T) {
	d := NewDNSChain()
	ip := net.ParseIP("203.0.113.7").To4()
	// Insert with already-expired entry.
	d.mu.Lock()
	d.cache["expired.example.com"] = cacheEntry{
		ips:    []net.IP{ip},
		expiry: time.Now().Add(-time.Minute),
	}
	d.mu.Unlock()

	// Force a non-owned-domain path by name; system resolver will be hit.
	// We can't assert on the resolver result without DNS, so just check
	// that the function bypassed the cache (returned no error or a
	// resolver-level error, but did NOT return the stale ip).
	got, err := d.Lookup(context.Background(), "expired.example.com")
	if err == nil && len(got) > 0 && got[0].Equal(ip) {
		t.Errorf("expected cache miss; got stale entry %v", got)
	}
}

func TestDialContextRejectsIPv6Literal(t *testing.T) {
	d := NewDNSChain()
	_, err := d.DialContext(context.Background(), "tcp", "[2606:4700::1]:443")
	if err == nil {
		t.Fatal("expected ipv6 literal to be rejected")
	}
}

func TestDialContextRejectsBadAddr(t *testing.T) {
	d := NewDNSChain()
	_, err := d.DialContext(context.Background(), "tcp", "not-a-host-port")
	if err == nil {
		t.Fatal("expected bad addr to be rejected")
	}
}

func TestSharedChainSingleton(t *testing.T) {
	a := SharedChain()
	b := SharedChain()
	if a != b {
		t.Errorf("SharedChain returned different instances %p vs %p", a, b)
	}
}

func TestTransportSetsDialContext(t *testing.T) {
	d := NewDNSChain()
	tr := d.Transport()
	if tr.DialContext == nil {
		t.Fatal("Transport().DialContext is nil")
	}
	if tr.ForceAttemptHTTP2 {
		t.Fatal("Transport() enabled HTTP/2")
	}
	if tr.TLSNextProto == nil || len(tr.TLSNextProto) != 0 {
		t.Fatalf("Transport().TLSNextProto = %#v, want non-nil empty map", tr.TLSNextProto)
	}
	if tr.TLSClientConfig == nil || len(tr.TLSClientConfig.NextProtos) != 1 || tr.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatalf("Transport().TLSClientConfig.NextProtos = %#v", tr.TLSClientConfig)
	}
	if tr.ResponseHeaderTimeout != 3*time.Minute {
		t.Fatalf("Transport().ResponseHeaderTimeout = %s", tr.ResponseHeaderTimeout)
	}
	if tr.IdleConnTimeout != 30*time.Second {
		t.Fatalf("Transport().IdleConnTimeout = %s", tr.IdleConnTimeout)
	}
}

func TestHTTPClientPreservesTimeout(t *testing.T) {
	d := NewDNSChain()
	want := 17 * time.Second
	client := d.HTTPClient(want)
	if client.Timeout != want {
		t.Fatalf("HTTPClient().Timeout = %s, want %s", client.Timeout, want)
	}
	if client.Transport == nil {
		t.Fatal("HTTPClient().Transport is nil")
	}
}

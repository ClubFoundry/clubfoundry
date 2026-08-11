package cloud

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// DialContext resolves hostnames through DNSChain and dials IPv4 only.
func (d *DNSChain) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("split %s: %w", addr, err)
	}

	dialer := &net.Dialer{Timeout: dialerTimeout}

	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return nil, fmt.Errorf("ipv6 disabled in sidecar dialer: %s", addr)
		}
		return dialer.DialContext(ctx, forceIPv4Network, addr)
	}

	ips, err := d.Lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup %s: %w", host, err)
	}

	var lastErr error
	for _, ip := range ips {
		target := net.JoinHostPort(ip.String(), port)
		conn, dErr := dialer.DialContext(ctx, forceIPv4Network, target)
		if dErr == nil {
			return conn, nil
		}
		lastErr = dErr
	}
	if lastErr == nil {
		lastErr = errors.New("no IPs returned")
	}
	return nil, fmt.Errorf("dial %s: %w", host, lastErr)
}

// Transport returns a fresh HTTP/1.1 transport using the IPv4 DNS chain.
func (d *DNSChain) Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = d.DialContext
	// All three settings are required to prevent HTTP/2 negotiation.
	t.ForceAttemptHTTP2 = false
	t.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{}
	} else {
		t.TLSClientConfig = t.TLSClientConfig.Clone()
	}
	t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	t.ResponseHeaderTimeout = 3 * time.Minute
	t.IdleConnTimeout = 30 * time.Second
	return t
}

// HTTPClient returns a client using the resilient owned-domain transport.
func (d *DNSChain) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Transport: d.Transport(), Timeout: timeout}
}

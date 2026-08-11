package cloud

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// raceResolve returns the first successful answer from the pinned resolvers.
func (d *DNSChain) raceResolve(ctx context.Context, host string) ([]net.IP, error) {
	type result struct {
		ips []net.IP
		err error
	}
	out := make(chan result, len(pinnedResolvers))

	raceCtx, cancel := context.WithTimeout(ctx, resolverReadTimeout)
	defer cancel()

	for _, addr := range pinnedResolvers {
		go func(resolverAddr string) {
			r := &net.Resolver{
				PreferGo: true,
				Dial: func(c context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{Timeout: resolverDialTimeout}).
						DialContext(c, "udp", resolverAddr)
				},
			}
			ips, err := r.LookupIP(raceCtx, "ip4", host)
			if err != nil {
				out <- result{err: fmt.Errorf("%s: %w", resolverAddr, err)}
				return
			}
			v4 := make([]net.IP, 0, len(ips))
			for _, ip := range ips {
				if ip.To4() != nil {
					v4 = append(v4, ip.To4())
				}
			}
			if len(v4) == 0 {
				out <- result{err: fmt.Errorf("%s: no A records", resolverAddr)}
				return
			}
			out <- result{ips: v4}
		}(addr)
	}

	errs := make([]string, 0, len(pinnedResolvers))
	for i := 0; i < len(pinnedResolvers); i++ {
		select {
		case r := <-out:
			if r.err == nil {
				return r.ips, nil
			}
			errs = append(errs, r.err.Error())
		case <-raceCtx.Done():
			errs = append(errs, raceCtx.Err().Error())
			return nil, fmt.Errorf("all resolvers failed: %s", strings.Join(errs, "; "))
		}
	}
	return nil, fmt.Errorf("all resolvers failed: %s", strings.Join(errs, "; "))
}

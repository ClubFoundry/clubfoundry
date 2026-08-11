package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
)

// mainHealthy reports whether one bounded health request returns HTTP 200.
func mainHealthy(ctx context.Context, url string, timeout time.Duration) bool {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := cloud.SharedChain().HTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitMainHealthy polls every two seconds until main is healthy or time expires.
func waitMainHealthy(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if mainHealthy(ctx, url, 3*time.Second) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("main /health did not return 200 within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

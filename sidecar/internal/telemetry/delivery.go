package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// deliverWithBackoff retries transient delivery failures until ctx expires.
func (r *Reporter) deliverWithBackoff(ctx context.Context, body []byte) {
	backoff := 5 * time.Second
	for attempt := 0; ; attempt++ {
		if err := r.deliverOnce(ctx, body); err == nil {
			return
		} else {
			fmt.Fprintf(os.Stderr, "telemetry: attempt %d failed: %v (retry in %s)\n", attempt+1, err, backoff)
		}
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "telemetry: giving up after %d attempts\n", attempt+1)
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
	}
}

func (r *Reporter) deliverOnce(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.CloudBaseURL+reportEndpointPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := r.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

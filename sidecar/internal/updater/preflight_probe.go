package updater

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
)

// preflightHeadTimeout bounds the download-URL connectivity probe.
const preflightHeadTimeout = 8 * time.Second

// freeDiskBytes is implemented per OS. Unsupported development hosts return
// an error and the readiness check degrades gracefully.

// probeURL accepts any response below HTTP 500 because only connectivity is
// checked here; the download path reports authorization and missing objects.
func probeURL(parent context.Context, url string) error {
	ctx, cancel := context.WithTimeout(parent, preflightHeadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := cloud.SharedChain().HTTPClient(preflightHeadTimeout).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

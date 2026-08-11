package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// fetchChannelManifest returns the first host that serves a manifest that is
// actually ours. Every source is validated, never trusted on status code
// alone: an object-storage host with a web-listing/SPA fallback answers 200
// with HTML for a missing key, and a 200 full of HTML decoding into a
// zero-valued struct would read as "no update" forever, which is worse than
// the outage it is meant to survive.
func fetchChannelManifest(ctx context.Context, hc *http.Client, channel string) (*ChannelManifest, error) {
	var lastErr error
	for _, u := range channelManifestURLs(channel) {
		m, err := fetchOneChannelManifest(ctx, hc, u, channel)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", u, err)
			continue
		}
		return m, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no manifest hosts configured")
	}
	return nil, lastErr
}

func fetchOneChannelManifest(ctx context.Context, hc *http.Client, u, channel string) (*ChannelManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChannelManifestBytes))
	if err != nil {
		return nil, err
	}

	var m ChannelManifest
	// DisallowUnknownFields is deliberately NOT set: the manifest is additive
	// like the rest of the wire, and a newer publisher adding a field must not
	// blind an older sidecar to the whole answer.
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("not json (%w)", err)
	}
	// Reject JSON that says nothing and stale or misrouted channel manifests.
	if m.Latest == "" {
		return nil, fmt.Errorf("no latest field — not our manifest")
	}
	if m.Channel != "" && !strings.EqualFold(m.Channel, channel) {
		return nil, fmt.Errorf("manifest is for channel %q, asked for %q", m.Channel, channel)
	}
	if m.DownloadSha256 == "" {
		// Without the anchor the download cannot be verified, and an
		// unverified image is not something to install off a fallback path.
		return nil, fmt.Errorf("manifest has no downloadSha256")
	}
	return &m, nil
}

package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) checkUpdatesViaWorker(ctx context.Context, currentVersion, channel string) (*UpdateResponse, error) {
	q := url.Values{}
	q.Set("current", currentVersion)
	if channel != "" {
		q.Set("channel", channel)
	}
	if c.Instance != "" {
		q.Set("instanceId", c.Instance)
	}
	endpoint := c.BaseURL + "/api/update?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloud request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud status %d", resp.StatusCode)
	}
	var out UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &out, nil
}

// FetchVersionMetadata returns metadata used to reinstall the running version.
func (c *Client) FetchVersionMetadata(ctx context.Context, version string) (*VersionMetadata, error) {
	if c.BaseURL == "" {
		return nil, nil
	}
	if version == "" {
		return nil, fmt.Errorf("FetchVersionMetadata: empty version")
	}
	endpoint := c.BaseURL + "/api/version/" + url.PathEscape(version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloud request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("version %q not found in cloud catalog", version)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud status %d", resp.StatusCode)
	}
	var out VersionMetadata
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &out, nil
}

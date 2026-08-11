package cloud

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PostAlert sends a best-effort sidecar halt or recovery notice.
// Server status codes are ignored because older cloud deployments may not
// implement this endpoint yet.
func (c *Client) PostAlert(ctx context.Context, reason string) error {
	if c.BaseURL == "" {
		return nil
	}
	body := fmt.Sprintf(`{"instance_id":%q,"reason":%q,"ts":%q}`,
		c.Instance, reason, time.Now().UTC().Format(time.RFC3339))
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.BaseURL+"/api/sidecar/alert",
		strings.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

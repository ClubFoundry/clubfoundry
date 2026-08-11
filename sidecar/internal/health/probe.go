package health

import (
	"context"
	"encoding/json"
	"net/http"
)

// Probe checks HTTP once and falls back to durable boot progress when the
// application has not bound its listener yet.
func (c *Checker) Probe(ctx context.Context) (bool, Report, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return false, Report{}, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		if bs := c.readBootState(); bs.Phase != "" {
			return false, Report{Status: "starting", Phase: bs.Phase, Migration: bs.Migration}, nil
		}
		return false, Report{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, Report{}, nil
	}
	var r Report
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return true, Report{Status: "ok"}, nil
	}
	return r.IsHealthy(), r, nil
}

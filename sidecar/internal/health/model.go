package health

// Report is the additive main-app /health response.
type Report struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Ready     bool   `json:"ready"`
	Phase     string `json:"phase"`
	Migration string `json:"migration"`
}

// IsHealthy accepts both status-only and explicit-readiness responses.
func (r Report) IsHealthy() bool {
	if r.Status == "ok" && r.Ready {
		return true
	}
	if r.Status == "ok" {
		return true
	}
	return false
}

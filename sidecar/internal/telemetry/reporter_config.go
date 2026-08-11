package telemetry

import (
	"net/http"
	"time"

	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/health"
)

// Reporter is the post-update telemetry client. It is safe for concurrent use.
type Reporter struct {
	CloudBaseURL string
	InstanceID   string
	Health       *health.Checker
	LogDir       string
	HTTPClient   *http.Client
	SettleWindow time.Duration
}

const (
	defaultSettleWindow = 3 * time.Minute
	reportEndpointPath  = "/api/update-report"
	userAgent           = "clubfoundry-updater-telemetry/1"
)

func (r *Reporter) settleWindow() time.Duration {
	if r.SettleWindow > 0 {
		return r.SettleWindow
	}
	return defaultSettleWindow
}

func (r *Reporter) logDir() string {
	if r.LogDir != "" {
		return r.LogDir
	}
	return "/app/data/update-logs"
}

func (r *Reporter) client() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return cloud.SharedChain().HTTPClient(30 * time.Second)
}

// Package cloud talks to the ClubFoundry Cloud /api/update endpoint to
// discover new versions and recall notices.
//
// The sidecar polls the update endpoint independently because the main
// application may be down exactly when a recall notice must still be handled.
package cloud

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Client queries the ClubFoundry update API for release and recovery metadata.
// An empty BaseURL keeps update discovery disabled for offline deployments.
type Client struct {
	BaseURL  string
	Instance string
	HTTP     *http.Client
}

// DefaultClient reads the cloud URL from env. An empty URL makes every
// call a no-op, which is the correct behavior for air-gapped installs.
// The HTTP client uses the shared DNS chain so /api/update goes over
// IPv4 with pinned resolvers — see dnschain.go.
func DefaultClient() *Client {
	return &Client{
		BaseURL:  os.Getenv("CLUBFOUNDRY_CLOUD_URL"),
		Instance: os.Getenv("CLUBFOUNDRY_INSTANCE_ID"),
		HTTP:     SharedChain().HTTPClient(30 * time.Second),
	}
}

// CheckUpdates queries the cloud for the latest version + recall list.
// Pass the running app's current version so the server can compute the
// update path. An empty BaseURL short-circuits with nil, nil.
//
// If the Worker is unreachable or returns an error, this falls back to the
// static per-channel manifest on the release
// mirrors (mirror.go). The fallback is narrow on purpose: it can only report a
// strictly newer build, never a recall verdict, a rollback or a critical flag.
// Those stay the Worker's. An empty BaseURL still short-circuits: that is an
// air-gapped install saying "do not phone home", not an outage.
func (c *Client) CheckUpdates(ctx context.Context, currentVersion, channel string) (*UpdateResponse, error) {
	if c.BaseURL == "" {
		return nil, nil
	}
	out, err := c.checkUpdatesViaWorker(ctx, currentVersion, channel)
	if err == nil {
		return out, nil
	}
	mirrored, mirrorErr := c.checkUpdatesViaMirror(ctx, currentVersion, channel)
	if mirrorErr != nil {
		// Report the Worker's failure, not the mirror's: the Worker is the
		// authority and its error is the one worth acting on. The mirror's is
		// appended so a reader can see the fallback was tried and why it also
		// failed, rather than wondering whether it ran at all.
		return nil, fmt.Errorf("%w (channel-manifest fallback also failed: %v)", err, mirrorErr)
	}
	return mirrored, nil
}

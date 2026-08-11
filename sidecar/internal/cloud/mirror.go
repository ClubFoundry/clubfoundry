// Static channel manifests preserve verified downloads during cloud API
// outages. The Worker remains authoritative for entitlements, recalls,
// rollbacks, critical flags, and multi-hop paths.
package cloud

import "context"

// checkUpdatesViaMirror answers CheckUpdates from the static manifest.
//
// It can advertise only a strictly newer verified build. Critical and rollback
// decisions require the Worker and are never inferred from a static snapshot.
func (c *Client) checkUpdatesViaMirror(ctx context.Context, currentVersion, channel string) (*UpdateResponse, error) {
	m, err := fetchChannelManifest(ctx, c.HTTP, channel)
	if err != nil {
		return nil, err
	}
	newer := IsStrictlyNewerAppVersion(currentVersion, m.Latest)
	return &UpdateResponse{
		Latest:                m.Latest,
		CurrentIsLatest:       !newer,
		Critical:              false,
		Recalled:              m.Recalled,
		DownloadUrl:           m.DownloadUrl,
		DownloadUrls:          m.DownloadUrls,
		DownloadSha256:        m.DownloadSha256,
		UpdaterVersion:        m.UpdaterVersion,
		UpdaterDownloadUrl:    m.UpdaterDownloadUrl,
		UpdaterDownloadUrls:   m.UpdaterDownloadUrls,
		UpdaterDownloadSha256: m.UpdaterDownloadSha256,
	}, nil
}

package cloud

// UpdateResponse is the subset of /api/update used by the sidecar.
type UpdateResponse struct {
	Latest          string   `json:"latest"`
	CurrentIsLatest bool     `json:"currentIsLatest"`
	Critical        bool     `json:"critical"`
	Recalled        []string `json:"recalled"`
	RollbackTo      string   `json:"rollbackTo,omitempty"`

	// The main-image archive may have one legacy URL or an ordered mirror set.
	DownloadUrl    string   `json:"downloadUrl,omitempty"`
	DownloadSha256 string   `json:"downloadSha256,omitempty"`
	DownloadUrls   []string `json:"downloadUrls,omitempty"`

	// Updater artifacts are versioned independently from the main image.
	UpdaterVersion        string   `json:"updaterVersion,omitempty"`
	UpdaterDownloadUrl    string   `json:"updaterDownloadUrl,omitempty"`
	UpdaterDownloadSha256 string   `json:"updaterDownloadSha256,omitempty"`
	UpdaterDownloadUrls   []string `json:"updaterDownloadUrls,omitempty"`

	UpdatePath *struct {
		From              string   `json:"from"`
		To                string   `json:"to"`
		Path              []string `json:"path"`
		DirectJumpAllowed bool     `json:"directJumpAllowed"`
	} `json:"updatePath,omitempty"`
}

// VersionMetadata describes a known release returned by /api/version/<v>.
type VersionMetadata struct {
	Version               string   `json:"version"`
	Maturity              string   `json:"maturity"`
	Channel               string   `json:"channel"`
	Recalled              bool     `json:"recalled"`
	ReleasedAt            string   `json:"releasedAt"`
	Changelog             string   `json:"changelog,omitempty"`
	DownloadUrl           string   `json:"downloadUrl,omitempty"`
	DownloadSha256        string   `json:"downloadSha256,omitempty"`
	DownloadUrls          []string `json:"downloadUrls,omitempty"`
	ImageSha256           string   `json:"imageSha256,omitempty"`
	UpdaterVersion        string   `json:"updaterVersion,omitempty"`
	UpdaterDownloadUrl    string   `json:"updaterDownloadUrl,omitempty"`
	UpdaterDownloadSha256 string   `json:"updaterDownloadSha256,omitempty"`
	UpdaterDownloadUrls   []string `json:"updaterDownloadUrls,omitempty"`
}

// IsRecalled reports whether currentVersion appears in the recall list.
func IsRecalled(resp *UpdateResponse, currentVersion string) bool {
	if resp == nil {
		return false
	}
	for _, version := range resp.Recalled {
		if version == currentVersion {
			return true
		}
	}
	return false
}

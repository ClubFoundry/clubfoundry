package cloud

// Public release hosts are ordered by preference and span independent
// providers. They must match the release publisher configuration. Generic site
// mirrors are invalid because they may return HTML for missing artifacts.
var channelManifestHosts = []string{
	"https://downloads.clubfoundry.net",
	"https://187e8e45-4730-40ea-9e73-d65480da7b1c.selstorage.ru",
}

// Reject responses too large to be a channel manifest.
const maxChannelManifestBytes = 64 * 1024

// ChannelManifest is the published shape of channels/<channel>.json. Fields
// mirror the version-independent half of UpdateResponse, which depends only on
// the channel and never on who is asking.
type ChannelManifest struct {
	Channel string `json:"channel"`
	Latest  string `json:"latest"`

	DownloadUrl    string   `json:"downloadUrl,omitempty"`
	DownloadUrls   []string `json:"downloadUrls,omitempty"`
	DownloadSha256 string   `json:"downloadSha256,omitempty"`

	UpdaterVersion        string   `json:"updaterVersion,omitempty"`
	UpdaterDownloadUrl    string   `json:"updaterDownloadUrl,omitempty"`
	UpdaterDownloadUrls   []string `json:"updaterDownloadUrls,omitempty"`
	UpdaterDownloadSha256 string   `json:"updaterDownloadSha256,omitempty"`

	Recalled    []string `json:"recalled,omitempty"`
	GeneratedAt string   `json:"generatedAt,omitempty"`
}

func channelManifestURLs(channel string) []string {
	if channel == "" {
		channel = "stable"
	}
	urls := make([]string, 0, len(channelManifestHosts))
	for _, h := range channelManifestHosts {
		urls = append(urls, h+"/channels/"+channel+".json")
	}
	return urls
}

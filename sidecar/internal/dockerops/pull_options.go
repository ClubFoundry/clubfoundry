package dockerops

import (
	"io"
	"time"
)

// PullOpts defines artifact sources, integrity metadata, and progress hooks.
type PullOpts struct {
	URL          string
	URLs         []string
	Sha256       string
	Timeout      time.Duration
	ProgressFn   ProgressCallback
	OnLoadStart  func()
	LogWriter    io.Writer
	DownloadSize int64
}

// ProgressCallback receives sampled byte progress. bytesTotal may be unknown.
type ProgressCallback func(bytesDownloaded, bytesTotal int64, bytesPerSec float64)

// DefaultDownloadTimeout bounds HTTP fetch, verification, and docker load.
const DefaultDownloadTimeout = 30 * time.Minute

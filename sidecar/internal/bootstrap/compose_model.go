package bootstrap

// ComposeParams contains every value interpolated by WriteComposeFile.
type ComposeParams struct {
	// Image references are resolved before the compose file is rendered.
	MainImage    string
	UpdaterImage string

	MainService    string
	UpdaterService string

	// Host paths are evaluated by the Docker daemon.
	HostDataDir     string
	HostComposeFile string

	// CloudURL keeps a compose-started sidecar on the configured control plane.
	CloudURL string
}

package updater

import (
	"context"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
)

// Docker abstracts the docker-cli operations the updater package needs.
// dockerops.Config implements it in production; tests use an in-memory fake so
// failure and ordering contracts do not require a Docker daemon.
type Docker interface {
	MainServiceName() string
	UpdaterServiceName() string
	Pull(ctx context.Context, service, tag string, opts dockerops.PullOpts) error
	Stop(ctx context.Context, service string) error
	Up(ctx context.Context, service string) error
	PS(ctx context.Context) ([]dockerops.ServiceInfo, error)
	TagImage(ctx context.Context, src, dst string) error
	HasImage(ctx context.Context, tag string) bool
	SpawnRecreateTrampoline(ctx context.Context, service string, delaySec int, opts dockerops.TrampolineOpts) error
	SetServiceImage(service, newRef string) error
	CurrentImageRef(service string) (string, error)
	// RunMigrationDryrun validates a database copy with the new image.
	RunMigrationDryrun(ctx context.Context, imageRef string, opts dockerops.DryrunOpts) error
	// ValidateComposeForRecreate checks local Compose inputs before mutation.
	ValidateComposeForRecreate(ctx context.Context, service string) error
}

// HealthChecker abstracts main application readiness checks.
type HealthChecker interface {
	Probe(ctx context.Context) (bool, health.Report, error)
	WaitHealthy(ctx context.Context, interval time.Duration, onProgress ...health.ProgressFn) (health.Report, error)
}

// Backup abstracts SQLite triplet backup operations.
type Backup interface {
	CreateBackup(fromVersion string) (string, error)
	RestoreBackup(path string) error
	LatestBackup() (string, error)
	PruneOld() error
}

// VersionMetadata avoids a package cycle while preserving the cloud contract.
type VersionMetadata struct {
	Version        string
	Recalled       bool
	DownloadUrl    string
	DownloadSha256 string
	DownloadUrls   []string
}

// CloudClient provides the metadata required for same-version reinstall.
type CloudClient interface {
	FetchVersionMetadata(ctx context.Context, version string) (*VersionMetadata, error)
}

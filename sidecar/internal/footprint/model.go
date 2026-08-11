package footprint

import "time"

// DataDirInside is the data-volume mount inside the sidecar container.
const DataDirInside = "/app/data"

// Report is the byte-based wire payload returned by /footprint.
type Report struct {
	GeneratedAt           time.Time             `json:"generated_at"`
	Repos                 map[string]RepoReport `json:"repos"`
	CFImagesTotalBytes    int64                 `json:"cf_images_total_bytes"`
	OtherDockerBytes      int64                 `json:"other_docker_bytes"`
	DockerImagesBytes     int64                 `json:"docker_images_bytes"`
	DockerBuildCacheBytes int64                 `json:"docker_buildcache_bytes"`
	DockerVolumesBytes    int64                 `json:"docker_volumes_bytes"`
	DataVolume            DiskStat              `json:"data_volume"`
	Errors                []string              `json:"errors,omitempty"`
}

// RepoReport contains newest-first image tags for one repository.
type RepoReport struct {
	TotalBytes  int64     `json:"total_bytes"`
	ImagesByTag []TagInfo `json:"images_by_tag"`
}

// TagInfo is one Docker image tag in a repository.
type TagInfo struct {
	Tag          string `json:"tag"`
	ID           string `json:"id"`
	SizeBytes    int64  `json:"size_bytes"`
	CreatedAtRaw string `json:"created_at_raw"`
}

// DiskStat reports data-volume filesystem usage in bytes.
type DiskStat struct {
	Path       string `json:"path"`
	TotalBytes int64  `json:"total_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
	FreeBytes  int64  `json:"free_bytes"`
	Available  bool   `json:"available"`
}

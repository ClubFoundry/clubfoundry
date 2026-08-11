// Package dockerops exposes read-only Docker footprint models and commands.

package dockerops

// ImageInfo is one entry from `docker images --format '{{json .}}'`.
// Wire shape mirrors the docker CLI; we expose only the fields the
// /footprint handler actually consumes.
type ImageInfo struct {
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	ID         string `json:"ID"`
	CreatedAt  string `json:"CreatedAt"`  // Docker CLI timestamp
	Size       string `json:"Size"`       // Docker CLI size, parsed by parseSizeString
	SizeBytes  int64  `json:"size_bytes"` // populated by ListImagesByRepo
}

// SystemDFEntry is one row of `docker system df --format '{{json .}}'`.
// Type is one of "Images" / "Containers" / "Local Volumes" / "Build Cache".
type SystemDFEntry struct {
	Type             string `json:"Type"`
	Size             string `json:"Size"`              // "5.281GB"
	Reclaimable      string `json:"Reclaimable"`       // "2.733GB (51%)"
	SizeBytes        int64  `json:"size_bytes"`        // populated by SystemDF
	ReclaimableBytes int64  `json:"reclaimable_bytes"` // populated by SystemDF
}

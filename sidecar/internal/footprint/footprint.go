// Package footprint reports ClubFoundry's disk usage on the TrueNAS host:
// per-version docker images, data volume content, and the host boot-pool
// (the latter is reported by the BACKEND via the TrueNAS API; this package
// returns only the docker + data-dir slices the sidecar can see directly).
// All functions are read-only and safe to call from the HTTP path.
package footprint

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// Collect builds the full Report. Each stage handles its own errors —
// a docker CLI hiccup on one repo does not block the rest.
//
// Repos to scan are passed in (typically ["clubfoundry","clubfoundry-updater"])
// — caller controls what's "ours" so naming changes don't require a new
// release of this package.
func Collect(ctx context.Context, dock dockerops.Config, repos []string) Report {
	rep := Report{
		GeneratedAt: time.Now().UTC(),
		Repos:       map[string]RepoReport{},
	}

	var cfTotal int64
	for _, repo := range repos {
		imgs, err := dock.ListImagesByRepo(ctx, repo)
		if err != nil {
			rep.Errors = append(rep.Errors, "list "+repo+": "+err.Error())
			continue
		}
		// Sort newest-first by CreatedAt string. Docker emits a stable
		// "YYYY-MM-DD HH:MM:SS …" prefix, so lexical reverse-sort is
		// chronologically correct.
		sort.Slice(imgs, func(i, j int) bool { return imgs[i].CreatedAt > imgs[j].CreatedAt })

		repoReport := buildRepoReport(imgs)
		rep.Repos[repo] = repoReport
		cfTotal += repoReport.TotalBytes
	}
	rep.CFImagesTotalBytes = cfTotal

	// Aggregate `docker system df` so we can report total docker footprint.
	if entries, err := dock.SystemDF(ctx); err == nil {
		images := dockerops.FindDFEntry(entries, "Images")
		buildcache := dockerops.FindDFEntry(entries, "Build Cache")
		volumes := dockerops.FindDFEntry(entries, "Local Volumes")
		rep.DockerImagesBytes = images.SizeBytes
		rep.DockerBuildCacheBytes = buildcache.SizeBytes
		rep.DockerVolumesBytes = volumes.SizeBytes
		// "Other docker" = everything not ours. Floor at zero — image dedup
		// can make CF images report > "total Images" via SharedSize accounting.
		other := images.SizeBytes - cfTotal + buildcache.SizeBytes + volumes.SizeBytes
		if other < 0 {
			other = 0
		}
		rep.OtherDockerBytes = other
	} else {
		rep.Errors = append(rep.Errors, "system df: "+err.Error())
	}

	// Data volume usage. Stat helper is build-tagged; on non-Linux dev
	// hosts it returns Available=false and the UI shows "—".
	rep.DataVolume = statDataVolume(DataDirInside)

	return rep
}

func buildRepoReport(imgs []dockerops.ImageInfo) RepoReport {
	images := make([]TagInfo, 0, len(imgs))
	byID := make(map[string]int, len(imgs))

	for index, img := range imgs {
		key := img.ID
		if key == "" {
			// Do not collapse malformed CLI rows that lack an image ID.
			key = "\x00missing-id-" + strconv.Itoa(index)
		}

		if existingIndex, ok := byID[key]; ok {
			existing := &images[existingIndex]
			if !containsString(existing.Tags, img.Tag) {
				existing.Tags = append(existing.Tags, img.Tag)
				sort.Strings(existing.Tags)
				existing.Tag = existing.Tags[0]
			}
			if img.SizeBytes > existing.SizeBytes {
				existing.SizeBytes = img.SizeBytes
			}
			continue
		}

		tags := []string{img.Tag}
		images = append(images, TagInfo{
			Tag:          img.Tag,
			Tags:         tags,
			ID:           img.ID,
			SizeBytes:    img.SizeBytes,
			CreatedAtRaw: img.CreatedAt,
		})
		byID[key] = len(images) - 1
	}

	var total int64
	for _, image := range images {
		total += image.SizeBytes
	}
	return RepoReport{TotalBytes: total, ImagesByTag: images}
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

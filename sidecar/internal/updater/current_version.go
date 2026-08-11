package updater

import (
	"context"
	"log"
	"strings"
)

// CurrentVersion prefers the running service tag and falls back to compose intent.
func (d *Deps) CurrentVersion(ctx context.Context) string {
	svcs, psErr := d.Docker.PS(ctx)
	if psErr == nil {
		for _, s := range svcs {
			if s.Service == d.Docker.MainServiceName() {
				if s.Tag != "" {
					return s.Tag
				}
				// Digest-only runtime references fall back to compose intent.
				break
			}
		}
	}
	// Project-label drift can hide the service from compose ps.
	imageRef, refErr := d.Docker.CurrentImageRef(d.Docker.MainServiceName())
	if refErr != nil {
		log.Printf("CurrentVersion: PS miss + YAML fallback failed (ps_err=%v, yaml_err=%v) — returning unknown",
			psErr, refErr)
		return "unknown"
	}
	tag := extractTagFromImageRef(imageRef)
	if tag == "" {
		log.Printf("CurrentVersion: YAML image=%q has no tag — returning unknown", imageRef)
		return "unknown"
	}
	log.Printf("CurrentVersion: PS miss for service=%s, falling back to YAML tag=%q",
		d.Docker.MainServiceName(), tag)
	return tag
}

// extractTagFromImageRef preserves registry ports and ignores digests.
func extractTagFromImageRef(image string) string {
	if at := strings.LastIndex(image, "@"); at >= 0 {
		image = image[:at]
	}
	colon := strings.LastIndex(image, ":")
	if colon < 0 {
		return ""
	}
	lastSlash := strings.LastIndex(image, "/")
	if colon <= lastSlash {
		// `:` belongs to the registry port, not the tag.
		return ""
	}
	return image[colon+1:]
}

func orLatest(s string) string {
	if s == "" {
		return "latest"
	}
	return s
}

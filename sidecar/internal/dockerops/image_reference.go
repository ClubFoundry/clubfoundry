package dockerops

import (
	"fmt"
	"strings"
)

// PreloadedImageRef resolves the tag used for the local-image early-skip check.
// This check trusts tags already present in the local Docker daemon.
func (c Config) PreloadedImageRef(service, tag string) (string, error) {
	if service == "" || tag == "" {
		return "", fmt.Errorf("PreloadedImageRef: service and tag required")
	}
	cur, err := c.CurrentImageRef(service)
	if err != nil {
		return "", err
	}
	return resolveImageRef(cur, tag), nil
}

// resolveImageRef preserves the current repository when newRef is a bare tag.
func resolveImageRef(currentRef, newRef string) string {
	if looksLikeFullRef(newRef) {
		return newRef
	}
	repo := stripTagOrDigest(currentRef)
	return repo + ":" + newRef
}

// looksLikeFullRef distinguishes repository references from version-only tags.
func looksLikeFullRef(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsRune(s, '/') {
		return true
	}
	colon := strings.IndexRune(s, ':')
	if colon <= 0 {
		return false
	}
	// Version tags may contain dots; repository names must contain a letter here.
	prefix := s[:colon]
	hasLetter := false
	for _, r := range prefix {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '_' {
			hasLetter = true
			break
		}
	}
	return hasLetter && !strings.ContainsRune(prefix, '.')
}

// stripTagOrDigest keeps registry ports while removing an image tag or digest.
func stripTagOrDigest(ref string) string {
	if at := strings.LastIndex(ref, "@"); at >= 0 {
		return ref[:at]
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		return ref[:lastColon]
	}
	return ref
}

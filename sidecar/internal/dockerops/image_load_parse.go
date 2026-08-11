package dockerops

import "regexp"

var loadedImageRe = regexp.MustCompile(`(?m)^Loaded image:\s+(\S+)`)

// parseLoadedImage returns the first imported repository tag.
func parseLoadedImage(out string) string {
	m := loadedImageRe.FindStringSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

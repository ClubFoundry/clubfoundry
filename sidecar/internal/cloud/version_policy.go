package cloud

import (
	"strconv"
	"strings"
)

// IsStrictlyNewerAppVersion reports whether advertised is strictly newer than
// current for the main app's MAJOR.MINOR.PATCH scheme.
//
// Only needed on the fallback path: normally the Worker computes
// currentIsLatest and the sidecar takes its word. Fail closed: anything that
// does not parse cleanly returns false, so a malformed manifest can never
// license an install, let alone a downgrade.
func IsStrictlyNewerAppVersion(current, advertised string) bool {
	cur, ok := parseAppVersion(current)
	if !ok {
		return false
	}
	adv, ok := parseAppVersion(advertised)
	if !ok {
		return false
	}
	for i := 0; i < 3; i++ {
		if adv[i] != cur[i] {
			return adv[i] > cur[i]
		}
	}
	return false
}

// parseAppVersion splits "1.3.122" into [1 3 122]. Strict: exactly three
// non-negative integer components, nothing else. No "v" prefix, pre-release
// suffix, or leading "+build" is accepted because the app publishes none.
func parseAppVersion(s string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

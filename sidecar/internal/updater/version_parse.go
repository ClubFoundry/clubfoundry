package updater

import (
	"strconv"
	"strings"
)

type sidecarVersionParts struct {
	major   int
	letters string
}

// parseSidecarVersion accepts `v<MAJOR>.<LETTERS>` and fails closed on any other form.
func parseSidecarVersion(s string) (sidecarVersionParts, bool) {
	if !strings.HasPrefix(s, "v") {
		return sidecarVersionParts{}, false
	}
	rest := s[1:]
	dot := strings.Index(rest, ".")
	if dot <= 0 {
		return sidecarVersionParts{}, false
	}
	majorStr := rest[:dot]
	// Reject leading zeros to keep the version canonical.
	if len(majorStr) > 1 && majorStr[0] == '0' {
		return sidecarVersionParts{}, false
	}
	major, err := strconv.Atoi(majorStr)
	if err != nil || major < 0 {
		return sidecarVersionParts{}, false
	}
	letters := rest[dot+1:]
	if letters == "" {
		return sidecarVersionParts{}, false
	}
	for _, c := range letters {
		if c < 'A' || c > 'Z' {
			return sidecarVersionParts{}, false
		}
	}
	return sidecarVersionParts{major: major, letters: letters}, true
}

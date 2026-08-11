package poller

import (
	"strings"
	"time"
)

// insideWindow reports whether now is inside an HH:MM-HH:MM UTC range.
func insideWindow(now time.Time, window string) bool {
	dash := strings.Index(window, "-")
	if dash < 0 {
		return false
	}
	start := parseHHMM(window[:dash])
	end := parseHHMM(window[dash+1:])
	if start < 0 || end < 0 {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if start <= end {
		return cur >= start && cur < end
	}
	return cur >= start || cur < end
}

func parseHHMM(s string) int {
	if len(s) != 5 || s[2] != ':' {
		return -1
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}

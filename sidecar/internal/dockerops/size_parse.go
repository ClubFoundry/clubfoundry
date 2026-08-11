package dockerops

import (
	"regexp"
	"strconv"
	"strings"
)

func parseSizeString(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0B" || s == "N/A" {
		return 0
	}
	i := len(s)
	for i > 0 && (s[i-1] >= 'A' && s[i-1] <= 'z') {
		i--
	}
	num := strings.TrimSpace(s[:i])
	unit := strings.TrimSpace(s[i:])
	if num == "" {
		return 0
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	mult := float64(0)
	switch unit {
	case "B":
		mult = 1
	case "kB":
		mult = 1e3
	case "MB":
		mult = 1e6
	case "GB":
		mult = 1e9
	case "TB":
		mult = 1e12
	default:
		return 0
	}
	return int64(f * mult)
}

var reclaimedRe = regexp.MustCompile(`(?i)Total[\s\w]*?reclaimed[\s\w]*?:\s*([0-9]+(?:\.[0-9]+)?)\s*([kKmMgGtT]?)B`)

func parseReclaimedBytes(out string) int64 {
	m := reclaimedRe.FindStringSubmatch(out)
	if len(m) < 3 {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	mult := 1.0
	switch strings.ToLower(m[2]) {
	case "k":
		mult = 1024
	case "m":
		mult = 1024 * 1024
	case "g":
		mult = 1024 * 1024 * 1024
	case "t":
		mult = 1024 * 1024 * 1024 * 1024
	}
	return int64(val * mult)
}

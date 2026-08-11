package health

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func dataDirOrDefault() string {
	d := os.Getenv("CLUBFOUNDRY_DATA_DIR")
	if d == "" {
		d = "/app/data"
	}
	return d
}

// ResolveMainHealthURL chooses the explicit override, shared .env port, or
// the conventional port 3000, in that order.
func ResolveMainHealthURL() string {
	if u := os.Getenv("CLUBFOUNDRY_HEALTH_URL"); u != "" {
		return u
	}
	port := mainPortFromEnvFile(filepath.Join(dataDirOrDefault(), ".env"))
	if port == "" {
		port = "3000"
	}
	return "http://127.0.0.1:" + port + "/health"
}

// mainPortFromEnvFile reads the leading numeric CLM_PORT value.
func mainPortFromEnvFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimPrefix(line, "export ")
		if !strings.HasPrefix(line, "CLM_PORT=") {
			continue
		}
		v := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "CLM_PORT=")), `"'`)
		i := 0
		for i < len(v) && v[i] >= '0' && v[i] <= '9' {
			i++
		}
		if i > 0 {
			return v[:i]
		}
	}
	return ""
}

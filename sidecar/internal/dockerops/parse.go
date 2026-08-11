package dockerops

import (
	"encoding/json"
	"strings"
)

// The docker compose v2 `ps --format json` line shape (relevant subset):
//
//	{
//	  "Name": "clubfoundry",
//	  "Image": "ghcr.io/clubfoundry/clubfoundry:1.0.30",
//	  "Service": "clubfoundry",
//	  "State": "running",
//	  "Status": "Up 3 hours"
//	}
type psLine struct {
	Service string `json:"Service"`
	Image   string `json:"Image"`
	State   string `json:"State"`
}

func decodePS(line string) (ServiceInfo, error) {
	var p psLine
	if err := json.Unmarshal([]byte(line), &p); err != nil {
		return ServiceInfo{}, err
	}
	return serviceInfoFromPSLine(p), nil
}

func parsePS(raw string) ([]ServiceInfo, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var entries []psLine
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, err
		}
		out := make([]ServiceInfo, 0, len(entries))
		for _, entry := range entries {
			out = append(out, serviceInfoFromPSLine(entry))
		}
		return out, nil
	}

	var out []ServiceInfo
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		service, err := decodePS(line)
		if err != nil {
			continue
		}
		out = append(out, service)
	}
	return out, nil
}

func serviceInfoFromPSLine(p psLine) ServiceInfo {
	return ServiceInfo{
		Service: p.Service,
		Image:   p.Image,
		Tag:     extractTag(p.Image),
		State:   p.State,
	}
}

func extractTag(image string) string {
	// Image shape: <registry>/<path>:<tag>@<digest> or <path>:<tag> or <path>
	if at := strings.LastIndex(image, "@"); at >= 0 {
		image = image[:at]
	}
	if colon := strings.LastIndex(image, ":"); colon >= 0 {
		// Guard against registry port colon (e.g. ghcr.io:443/foo/bar).
		// A tag comes after the last slash; if `:` is before it, it's not a tag.
		lastSlash := strings.LastIndex(image, "/")
		if colon > lastSlash {
			return image[colon+1:]
		}
	}
	return ""
}

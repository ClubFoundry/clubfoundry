package bootstrap

import (
	"encoding/json"
	"log"
)

// LogDriftReport emits the structured report and one line per finding/error.
func LogDriftReport(r DriftReport) {
	if len(r.Findings) == 0 && len(r.Errors) == 0 {
		return
	}
	body, _ := json.Marshal(r)
	log.Printf("[compose-drift] %s", string(body))
	for _, f := range r.Findings {
		log.Printf("[compose-drift] WARN kind=%s service=%s container=%s project=%q state=%s — %s",
			f.Kind, f.Service, f.ContainerName, f.ProjectLabel, f.State, f.Recommendation)
	}
	for _, e := range r.Errors {
		log.Printf("[compose-drift] ERROR %s", e)
	}
}

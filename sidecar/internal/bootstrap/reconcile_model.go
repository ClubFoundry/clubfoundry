package bootstrap

import "time"

// DriftReport describes compose containers outside the canonical project.
type DriftReport struct {
	CanonicalProject string         `json:"canonical_project"`
	Services         []string       `json:"services_checked"`
	Findings         []DriftFinding `json:"findings,omitempty"`
	Errors           []string       `json:"errors,omitempty"`
	CheckedAt        time.Time      `json:"checked_at"`
}

// DriftFinding is one detected topology discrepancy.
type DriftFinding struct {
	Service        string `json:"service"`
	ContainerID    string `json:"container_id"`
	ContainerName  string `json:"container_name"`
	ProjectLabel   string `json:"project_label"`
	State          string `json:"state"`
	ImageRef       string `json:"image_ref"`
	Kind           string `json:"kind"` // wrong_project | duplicate
	Recommendation string `json:"recommendation"`
}

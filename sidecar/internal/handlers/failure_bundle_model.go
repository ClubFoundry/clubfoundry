package handlers

type failureBundleSummary struct {
	Filename    string `json:"filename"`
	SizeBytes   int64  `json:"size_bytes"`
	ModifiedAt  int64  `json:"modified_at"`
	UpdateID    string `json:"update_id,omitempty"`
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
	Outcome     string `json:"outcome,omitempty"`
	Source      string `json:"source,omitempty"`
}

type failureBundleHeadline struct {
	UpdateID    string `json:"update_id"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Outcome     string `json:"outcome"`
	Source      string `json:"source"`
}

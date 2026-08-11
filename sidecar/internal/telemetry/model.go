package telemetry

// UpdateReport is the additive wire shape sent to the Worker.
type UpdateReport struct {
	ReportVersion  int          `json:"report_version"`
	InstanceID     string       `json:"instance_id"`
	UpdateID       string       `json:"update_id"`
	FromVersion    string       `json:"from_version"`
	ToVersion      string       `json:"to_version"`
	Outcome        string       `json:"outcome"`
	StartedAt      string       `json:"started_at"`
	FinishedAt     string       `json:"finished_at"`
	DurationMS     int64        `json:"duration_ms"`
	ErrorText      string       `json:"error,omitempty"`
	SettleAfterSec int64        `json:"settle_after_sec"`
	SettleResult   string       `json:"settle_result"`
	Steps          []ReportStep `json:"steps,omitempty"`
	Log            string       `json:"log,omitempty"`
}

// ReportStep is one version hop included in a settled update report.
type ReportStep struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Outcome  string `json:"outcome"`
	Duration int64  `json:"duration_ms"`
	Error    string `json:"error,omitempty"`
}

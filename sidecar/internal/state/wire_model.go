package state

// StepInfo describes the current hop in a stepped update.
type StepInfo struct {
	Index       int    `json:"index"`
	Total       int    `json:"total"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// DownloadProgress describes the latest in-memory download sample.
type DownloadProgress struct {
	BytesDownloaded int64   `json:"bytes_downloaded"`
	BytesTotal      int64   `json:"bytes_total"`
	BytesPerSecond  float64 `json:"bytes_per_second"`
	ETASeconds      int64   `json:"eta_seconds"`
}

// ErrorInfo contains a machine-readable code and diagnostic detail.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Snapshot is the additive-only wire format returned by /status.
type Snapshot struct {
	Kind              Kind              `json:"kind,omitempty"`
	Phase             Phase             `json:"phase"`
	SubStep           SubStep           `json:"sub_step,omitempty"`
	Detail            string            `json:"detail,omitempty"`
	LastError         string            `json:"last_error,omitempty"`
	LastErrorCode     string            `json:"last_error_code,omitempty"`
	SinceEpoch        int64             `json:"since_epoch"`
	StartedEpoch      int64             `json:"started_epoch,omitempty"`
	UpdateID          string            `json:"update_id,omitempty"`
	OpID              string            `json:"op_id,omitempty"`
	TargetVersion     string            `json:"target_version,omitempty"`
	StagedTarget      string            `json:"staged_target,omitempty"`
	CancelRequested   bool              `json:"cancel_requested,omitempty"`
	Step              *StepInfo         `json:"step,omitempty"`
	Download          *DownloadProgress `json:"download,omitempty"`
	PendingMainTarget string            `json:"pending_main_target,omitempty"`
}

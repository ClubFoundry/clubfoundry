package state

import (
	"sync"
	"time"
)

// Kind identifies an independently persisted operation flow.
type Kind string

const (
	KindMain Kind = "main"
	KindSelf Kind = "self"
)

// Phase is the coarse state of a sidecar-managed operation.
type Phase string

const (
	Idle        Phase = "idle"
	Checking    Phase = "checking"
	Staging     Phase = "staging"
	Staged      Phase = "staged"
	Updating    Phase = "updating"
	Cancelling  Phase = "cancelling"
	RollingBack Phase = "rolling_back"
	Error       Phase = "error"
)

// SubStep identifies work within an active phase.
type SubStep string

const (
	SubStepNone        SubStep = ""
	SubStepResolving   SubStep = "resolving"
	SubStepPreflight   SubStep = "preflight"
	SubStepStopping    SubStep = "stopping"
	SubStepBackup      SubStep = "backup"
	SubStepDownloading SubStep = "downloading"
	SubStepVerifying   SubStep = "verifying"
	SubStepLoading     SubStep = "loading"
	SubStepStarting    SubStep = "starting"
	SubStepMigrating   SubStep = "migrating"
	SubStepHealthCheck SubStep = "health_check"
	SubStepReporting   SubStep = "reporting"
	SubStepSpawning    SubStep = "spawning_trampoline"
)

// State provides synchronized access to one operation flow.
type State struct {
	mu                sync.Mutex
	kind              Kind
	storePath         string
	persistErr        error
	phase             Phase
	subStep           SubStep
	started           time.Time
	startedOp         time.Time
	detail            string
	lastErr           ErrorInfo
	step              *StepInfo
	download          *DownloadProgress
	updateID          string
	opID              string
	targetVer         string
	stagedTarget      string
	cancelRequested   bool
	pendingMainTarget string
	onChange          func(Snapshot)
}

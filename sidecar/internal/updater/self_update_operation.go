package updater

import (
	"fmt"
	"io"
	"time"

	"github.com/clubfoundry/updater/internal/state"
)

type selfUpdateOperation struct {
	state       *state.State
	started     time.Time
	updateID    string
	opID        string
	fromVersion string
	toVersion   string
	log         *updateLog
	logWriter   io.Writer
}

func (d *Deps) beginSelfUpdate(targetVersion, artifactURL string) *selfUpdateOperation {
	started := time.Now()
	fromVersion := d.SelfVersion
	if fromVersion == "" {
		fromVersion = "(sidecar)"
	}
	toVersion := targetVersion
	if toVersion == "" {
		toVersion = "latest"
	}

	op := &selfUpdateOperation{
		state:       d.selfStateOrFallback(),
		started:     started,
		updateID:    fmt.Sprintf("self-%d", started.UnixNano()),
		opID:        newOpID(),
		fromVersion: fromVersion,
		toVersion:   toVersion,
	}
	op.log = d.openUpdateLog(op.updateID, op.opID, state.KindSelf)
	op.logWriter = op.log.writer()
	op.log.writeStateSnapshot("state-pre", op.state.Snapshot())
	op.state.RegisterChangeHook(op.log.hookFn())
	fmt.Fprintf(op.logWriter, "=== Self-update %s started %s ===\n", op.updateID, started.Format(time.RFC3339))
	fmt.Fprintf(op.logWriter, "from=%q to=%q op_id=%q artifact_url=%q\n",
		op.fromVersion, op.toVersion, op.opID, artifactURL)
	return op
}

func (op *selfUpdateOperation) close() {
	op.state.RegisterChangeHook(nil)
	op.log.writeStateSnapshot("state-post", op.state.Snapshot())
	op.log.close()
}

func (d *Deps) selfStateOrFallback() *state.State {
	if d.SelfState != nil {
		return d.SelfState
	}
	return d.State
}

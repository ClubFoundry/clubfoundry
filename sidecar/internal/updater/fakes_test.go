package updater

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/health"
)

// fakeDocker is the fault-injection test double for the Docker interface.
//
// Behaviour knobs are plain fields (zero-value = "succeed"); failure modes
// are configured by setting the corresponding `*Err` field, an after-N-call
// counter, or a callback. The fake records every call into `calls` so tests
// can assert exact ordering (e.g. "Stop must be called before Backup").
//
// Concurrency: all mutator paths take fakeDocker.mu so harness scenarios
// that exercise cancel-mid-pull race-conditions don't corrupt the call log.
type fakeDocker struct {
	mu sync.Mutex

	main    string // returned by MainServiceName(); default "clubfoundry"
	updater string // returned by UpdaterServiceName(); default "clubfoundry-updater"

	// Per-op failure injection. Nil = succeed.
	pullErr     error
	stopErr     error
	upErr       error
	psErr       error
	tagErr      error
	trampErr    error
	setImageErr error
	dryrunErr   error
	validateErr error // ValidateComposeForRecreate

	// pullDelay simulates a slow network pull. Test runs see the delay
	// only when Pull is invoked under a context with a deadline shorter
	// than the delay — useful for cancel-mid-pull scenarios.
	pullDelay time.Duration

	// onPull is invoked synchronously inside Pull before any error/return.
	// Lets a scenario inspect opts.URL/Sha256 or change state mid-call.
	onPull func(opts dockerops.PullOpts)

	// psResult is what PS() returns (alongside psErr). Tests use this to
	// stage what CurrentVersion() will resolve to.
	psResult []dockerops.ServiceInfo

	// hasImage controls HasImage() lookup table. nil = always false.
	hasImage map[string]bool

	// imageRef is the value CurrentImageRef returns; default "clubfoundry:latest".
	imageRef string

	// calls records every method invocation in order. One entry per call,
	// formatted like "Pull(svc=clubfoundry, tag=1.1.80, sha=abc…)".
	calls []string
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		main:     "clubfoundry",
		updater:  "clubfoundry-updater",
		imageRef: "clubfoundry:latest",
		hasImage: map[string]bool{},
	}
}

func (f *fakeDocker) record(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeDocker) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeDocker) MainServiceName() string    { return f.main }
func (f *fakeDocker) UpdaterServiceName() string { return f.updater }

func (f *fakeDocker) Pull(ctx context.Context, service, tag string, opts dockerops.PullOpts) error {
	f.record(fmt.Sprintf("Pull(svc=%s,tag=%s,url=%q,size=%d)", service, tag, opts.URL, opts.DownloadSize))
	if f.onPull != nil {
		f.onPull(opts)
	}
	if f.pullDelay > 0 {
		select {
		case <-time.After(f.pullDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.pullErr
}

func (f *fakeDocker) Stop(ctx context.Context, service string) error {
	f.record(fmt.Sprintf("Stop(%s)", service))
	return f.stopErr
}

func (f *fakeDocker) Up(ctx context.Context, service string) error {
	f.record(fmt.Sprintf("Up(%s)", service))
	return f.upErr
}

func (f *fakeDocker) PS(ctx context.Context) ([]dockerops.ServiceInfo, error) {
	f.record("PS()")
	return f.psResult, f.psErr
}

func (f *fakeDocker) TagImage(ctx context.Context, src, dst string) error {
	f.record(fmt.Sprintf("TagImage(%s,%s)", src, dst))
	return f.tagErr
}

func (f *fakeDocker) HasImage(ctx context.Context, tag string) bool {
	f.record(fmt.Sprintf("HasImage(%s)", tag))
	return f.hasImage[tag]
}

func (f *fakeDocker) SpawnRecreateTrampoline(ctx context.Context, service string, delaySec int, opts dockerops.TrampolineOpts) error {
	f.record(fmt.Sprintf("SpawnRecreateTrampoline(%s,delay=%ds,sentinel=%s,target=%s)",
		service, delaySec, opts.SentinelPath, opts.TargetVersion))
	return f.trampErr
}

func (f *fakeDocker) SetServiceImage(service, newRef string) error {
	f.record(fmt.Sprintf("SetServiceImage(%s,%s)", service, newRef))
	return f.setImageErr
}

func (f *fakeDocker) CurrentImageRef(service string) (string, error) {
	f.record(fmt.Sprintf("CurrentImageRef(%s)", service))
	return f.imageRef, nil
}

func (f *fakeDocker) RunMigrationDryrun(ctx context.Context, imageRef string, opts dockerops.DryrunOpts) error {
	f.record(fmt.Sprintf("RunMigrationDryrun(%s,host=%s,src=%s,copy=%s)",
		imageRef, opts.HostDataDir, opts.SourceDBFile, opts.CopyDBFile))
	return f.dryrunErr
}

func (f *fakeDocker) ValidateComposeForRecreate(ctx context.Context, service string) error {
	f.record(fmt.Sprintf("ValidateComposeForRecreate(%s)", service))
	return f.validateErr
}

// fakeHealth fakes the HealthChecker interface. Each Probe() call advances
// the script — a slice of canned responses. WaitHealthy() consumes from the
// same script until it sees a healthy report or the script runs out (then
// errors with context-deadline-exceeded behaviour).
type fakeHealth struct {
	mu sync.Mutex

	// script is consumed in FIFO order by every Probe / WaitHealthy call.
	// Last entry repeats forever (useful for a steady-state result).
	script []healthProbeResult

	// probeErr is returned from EVERY Probe call as the third return —
	// dominates the script when non-nil. Used to simulate "process died".
	probeErr error

	// waitTimeout, when non-zero, makes WaitHealthy return ctx.Err()
	// instead of advancing the script — simulates timeout.
	waitTimeout bool

	calls int // number of Probe calls served
}

type healthProbeResult struct {
	ok     bool
	report health.Report
}

func newFakeHealth(seq ...healthProbeResult) *fakeHealth {
	return &fakeHealth{script: seq}
}

// withVersion is a convenience for the most common script: "ok with this
// version" repeated forever.
func newFakeHealthOK(version string) *fakeHealth {
	return newFakeHealth(healthProbeResult{
		ok:     true,
		report: health.Report{Status: "ok", Ready: true, Version: version},
	})
}

func (f *fakeHealth) next() healthProbeResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.script) == 0 {
		return healthProbeResult{}
	}
	if f.calls > len(f.script) {
		return f.script[len(f.script)-1]
	}
	return f.script[f.calls-1]
}

func (f *fakeHealth) Probe(ctx context.Context) (bool, health.Report, error) {
	if f.probeErr != nil {
		return false, health.Report{}, f.probeErr
	}
	r := f.next()
	return r.ok, r.report, nil
}

func (f *fakeHealth) WaitHealthy(ctx context.Context, interval time.Duration, onProgress ...health.ProgressFn) (health.Report, error) {
	if f.waitTimeout {
		return health.Report{}, fmt.Errorf("wait healthy: %w", context.DeadlineExceeded)
	}
	if f.probeErr != nil {
		return health.Report{}, fmt.Errorf("wait healthy: %w", f.probeErr)
	}
	// Drain the script until we find a healthy one or run out.
	for i := 0; i < 100; i++ {
		r := f.next()
		for _, fn := range onProgress {
			if fn != nil {
				fn(r.report)
			}
		}
		if r.ok {
			return r.report, nil
		}
	}
	return health.Report{}, fmt.Errorf("wait healthy: %w", context.DeadlineExceeded)
}

// fakeBackup fakes the Backup interface. Records the sequence of Create
// + Restore calls so tests can assert "rollback restored the right backup
// file". Defaults: backups succeed, latest backup is the most recent
// CreateBackup result.
type fakeBackup struct {
	mu sync.Mutex

	createErr  error
	restoreErr error
	pruneErr   error

	created []string // backup paths returned by CreateBackup, in order
	latest  string   // optional explicit LatestBackup result
	calls   []string
}

func newFakeBackup() *fakeBackup {
	return &fakeBackup{}
}

func (f *fakeBackup) record(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeBackup) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeBackup) CreateBackup(fromVersion string) (string, error) {
	f.record(fmt.Sprintf("CreateBackup(%s)", fromVersion))
	if f.createErr != nil {
		return "", f.createErr
	}
	path := fmt.Sprintf("/fake/backup/clm.db.pre-update-v%s-%d", fromVersion, len(f.created)+1)
	f.mu.Lock()
	f.created = append(f.created, path)
	f.mu.Unlock()
	return path, nil
}

func (f *fakeBackup) RestoreBackup(path string) error {
	f.record(fmt.Sprintf("RestoreBackup(%s)", path))
	return f.restoreErr
}

func (f *fakeBackup) LatestBackup() (string, error) {
	f.record("LatestBackup()")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.latest != "" {
		return f.latest, nil
	}
	if len(f.created) == 0 {
		return "", nil
	}
	return f.created[len(f.created)-1], nil
}

func (f *fakeBackup) PruneOld() error {
	f.record("PruneOld()")
	return f.pruneErr
}

// errFakeBoom is the canonical "we asked the fake to fail" sentinel.
var errFakeBoom = errors.New("fake_boom")

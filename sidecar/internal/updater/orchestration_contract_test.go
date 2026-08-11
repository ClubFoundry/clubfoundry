package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/history"
	"github.com/clubfoundry/updater/internal/state"
)

func TestCancellationClassificationContract(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if isCancelled(nil, context.Background(), context.Background()) {
			t.Fatal("nil error must not be classified as cancellation")
		}
	})

	t.Run("operator cancels inner context", func(t *testing.T) {
		parent := context.Background()
		inner, cancel := context.WithCancel(parent)
		cancel()
		if !isCancelled(context.Canceled, parent, inner) {
			t.Fatal("inner-only cancellation must be classified as operator cancellation")
		}
	})

	t.Run("parent cancellation", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		inner, innerCancel := context.WithCancel(parent)
		cancel()
		innerCancel()
		if isCancelled(context.Canceled, parent, inner) {
			t.Fatal("parent cancellation must remain an external failure")
		}
	})

	t.Run("error while contexts are active", func(t *testing.T) {
		if isCancelled(errors.New("pull failed"), context.Background(), context.Background()) {
			t.Fatal("ordinary errors must not be classified as cancellation")
		}
	})
}

func TestCancelLifecycleContract(t *testing.T) {
	deps := &Deps{State: state.New()}
	ctx, cancel := context.WithCancel(context.Background())
	deps.armCancel(cancel)
	if !deps.Cancel() {
		t.Fatal("armed operation must accept cancellation")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
	}
	deps.disarmCancel()
	if deps.Cancel() {
		t.Fatal("disarmed operation must report no cancellation target")
	}
}

func TestUpdateErrorClassificationContract(t *testing.T) {
	tests := []struct {
		name        string
		updateErr   error
		rollbackErr error
		want        string
	}{
		{"success", nil, nil, ""},
		{"rollback failed", errors.New("update failed"), errors.New("rollback failed"), "UPDATE_AND_ROLLBACK_FAILED"},
		{"disk", errors.New("INSUFFICIENT_DISK: required"), nil, "INSUFFICIENT_DISK"},
		{"network", errors.New("NETWORK_UNREACHABLE: mirror"), nil, "NETWORK_UNREACHABLE"},
		{"version", errors.New("version mismatch"), nil, "VERSION_MISMATCH"},
		{"checksum", errors.New("sha256 mismatch"), nil, "SHA256_MISMATCH"},
		{"deadline", context.DeadlineExceeded, nil, "DOWNLOAD_TIMEOUT"},
		{"health", errors.New("post-update health failed"), nil, "HEALTH_TIMEOUT"},
		{"container conflict", errors.New("already in use by container"), nil, "CONTAINER_NAME_CONFLICT"},
		{"pull", errors.New("pull image failed"), nil, "IMAGE_PULL_FAILED"},
		{"compose validation", errors.New("compose pre-flight failed"), nil, "COMPOSE_VALIDATION_FAILED"},
		{"compose up", errors.New("docker compose up failed"), nil, "COMPOSE_UP_FAILED"},
		{"backup", errors.New("backup db failed"), nil, "BACKUP_FAILED"},
		{"unknown", errors.New("unexpected"), nil, "UNKNOWN_ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyError(tc.updateErr, tc.rollbackErr); got != tc.want {
				t.Fatalf("classifyError(%v, %v) = %q, want %q", tc.updateErr, tc.rollbackErr, got, tc.want)
			}
		})
	}
}

func TestImageTagExtractionContract(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"repo/app:v1", "v1"},
		{"repo/app:v1@sha256:deadbeef", "v1"},
		{"registry.example:5000/repo/app:v2", "v2"},
		{"registry.example:5000/repo/app", ""},
		{"repo/app", ""},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s", tc.ref), func(t *testing.T) {
			if got := extractTagFromImageRef(tc.ref); got != tc.want {
				t.Fatalf("extractTagFromImageRef(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestExpectedVersionContract(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"", ""},
		{"latest", ""},
		{"unknown", ""},
		{"current", ""},
		{"previous", ""},
		{"1.2.3", "1.2.3"},
		{"clubfoundry:1.2.3", "1.2.3"},
		{"registry.example:5000/repo/clubfoundry:1.2.3", "1.2.3"},
		{"registry.example/repo/clubfoundry:1.2.3@sha256:deadbeef", "1.2.3"},
		{"registry.example:5000/repo/clubfoundry", ""},
		{"registry.example/repo/clubfoundry@sha256:deadbeef", ""},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := expectedVersion(tt.target); got != tt.want {
				t.Fatalf("expectedVersion(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestRollbackImagePriorityContract(t *testing.T) {
	tests := []struct {
		name       string
		local      map[string]bool
		wantCalls  []string
		rejectCall string
	}{
		{
			name: "exact versioned tag",
			local: map[string]bool{
				"clubfoundry:1.1.79":   true,
				"clubfoundry:previous": true,
			},
			wantCalls: []string{
				"HasImage(clubfoundry:1.1.79)",
				"SetServiceImage(clubfoundry,clubfoundry:1.1.79)",
			},
			rejectCall: "HasImage(clubfoundry:previous)",
		},
		{
			name: "retained previous fallback",
			local: map[string]bool{
				"clubfoundry:previous": true,
			},
			wantCalls: []string{
				"HasImage(clubfoundry:1.1.79)",
				"HasImage(clubfoundry:previous)",
				"SetServiceImage(clubfoundry,clubfoundry:previous)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, docker, _, _, _, cleanup := makeDeps(t)
			defer cleanup()
			docker.hasImage = tt.local
			deps.Health = healthRollbackOnly("1.1.79")

			if err := deps.doRollback(context.Background(), "backup.db", "1.1.79", io.Discard); err != nil {
				t.Fatalf("rollback failed: %v", err)
			}
			calls := docker.Calls()
			if err := containsAllInOrder(calls, tt.wantCalls); err != nil {
				t.Fatal(err)
			}
			for _, call := range calls {
				if call == tt.rejectCall {
					t.Fatalf("unexpected call %q in %v", call, calls)
				}
			}
		})
	}
}

func TestRunRollbackOperatorContract(t *testing.T) {
	deps, docker, _, backup, st, cleanup := makeDeps(t)
	defer cleanup()
	stagePSResult(docker, "1.1.80")
	backup.latest = "/fake/backup/clm.db.pre-update-v1.1.79-20260808T101426Z"
	docker.hasImage["clubfoundry:1.1.79"] = true
	deps.Health = healthRollbackOnly("1.1.79")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deps.RunRollback(ctx); err != nil {
		t.Fatalf("operator rollback: %v", err)
	}

	if got := st.Snapshot().Phase; got != state.Idle {
		t.Fatalf("phase = %s, want Idle", got)
	}
	if err := containsAllInOrder(docker.Calls(), []string{
		"PS()",
		"Stop(clubfoundry)",
		"HasImage(clubfoundry:1.1.79)",
		"SetServiceImage(clubfoundry,clubfoundry:1.1.79)",
		"Up(clubfoundry)",
	}); err != nil {
		t.Fatal(err)
	}
	if err := containsAllInOrder(backup.Calls(), []string{"LatestBackup()"}); err != nil {
		t.Fatal(err)
	}

	entries, err := deps.History.List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Outcome != history.OutcomeRollback || entries[0].FromVersion != "1.1.80" || entries[0].ToVersion != "1.1.79" {
		t.Fatalf("rollback history = %+v, want one rollback entry from 1.1.80 to 1.1.79", entries)
	}
}

func TestRunRollbackRejectsUnsafeBackupBeforeDestructiveCalls(t *testing.T) {
	tests := []struct {
		name       string
		latest     string
		wantErrSub string
	}{
		{
			name:       "missing backup",
			wantErrSub: "no pre-update backup available",
		},
		{
			name:       "non-concrete backup version",
			latest:     "/fake/backup/clm.db.pre-update-vunknown-20260808T101426Z",
			wantErrSub: "is not a concrete version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, docker, _, backup, _, cleanup := makeDeps(t)
			defer cleanup()
			stagePSResult(docker, "1.1.80")
			backup.latest = tt.latest

			err := deps.RunRollback(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("RunRollback error = %v, want substring %q", err, tt.wantErrSub)
			}
			for _, call := range docker.Calls() {
				if strings.HasPrefix(call, "Stop(") || strings.HasPrefix(call, "Up(") || strings.HasPrefix(call, "SetServiceImage(") {
					t.Fatalf("unsafe-backup rollback made destructive Docker call %q", call)
				}
			}
		})
	}
}

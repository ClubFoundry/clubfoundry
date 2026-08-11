package updater

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clubfoundry/updater/internal/state"
)

func TestTrampolineLogPathsContract(t *testing.T) {
	stdout, stderr := trampolineLogPaths("")
	if stdout != "" || stderr != "" {
		t.Fatalf("empty paths = %q/%q", stdout, stderr)
	}

	stdout, stderr = trampolineLogPaths(filepath.Join("logs", "self-1"))
	if stdout != filepath.Join("logs", "self-1", "trampoline.stdout") || stderr != filepath.Join("logs", "self-1", "trampoline.stderr") {
		t.Fatalf("log paths = %q/%q", stdout, stderr)
	}
}

func TestOperationIDContract(t *testing.T) {
	assertRandomID := func(t *testing.T, value, prefix string, byteCount int) {
		t.Helper()
		if !strings.HasPrefix(value, prefix) {
			t.Fatalf("id %q missing prefix %q", value, prefix)
		}
		raw := strings.TrimPrefix(value, prefix)
		decoded, err := hex.DecodeString(raw)
		if err != nil || len(decoded) != byteCount {
			t.Fatalf("id %q payload = %x, err=%v, want %d bytes", value, decoded, err, byteCount)
		}
	}

	opID := newOpID()
	trampolineID := newTrampolineID()
	assertRandomID(t, opID, "op-", 16)
	assertRandomID(t, trampolineID, "tramp-", 12)
}

func TestSelfStateFallbackContract(t *testing.T) {
	mainState := state.NewKindAware(state.KindMain, "")
	selfState := state.NewKindAware(state.KindSelf, "")
	if got := (&Deps{State: mainState}).selfStateOrFallback(); got != mainState {
		t.Fatal("missing self state did not fall back to main state")
	}
	if got := (&Deps{State: mainState, SelfState: selfState}).selfStateOrFallback(); got != selfState {
		t.Fatal("explicit self state was not selected")
	}
}

func TestSteppedUpdateRejectsEmptyPathContract(t *testing.T) {
	err := (&Deps{}).RunSteppedUpdate(context.Background(), nil)
	if err == nil || err.Error() != "empty update path" {
		t.Fatalf("error = %v, want empty update path", err)
	}
}

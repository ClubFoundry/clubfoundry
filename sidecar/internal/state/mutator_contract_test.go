package state

import "testing"

func TestSnapshotProgressIsolationContract(t *testing.T) {
	s := New()
	step := &StepInfo{Index: 1, Total: 3, ToVersion: "1.2.3"}
	download := DownloadProgress{BytesDownloaded: 100, BytesTotal: 1000}
	s.UpdateStep(step)
	s.UpdateDownload(download)

	step.Index = 9
	download.BytesDownloaded = 900
	first := s.Snapshot()
	if first.Step == nil || first.Step.Index != 1 || first.Download == nil || first.Download.BytesDownloaded != 100 {
		t.Fatalf("snapshot = %+v, want copied input values", first)
	}

	first.Step.Index = 7
	first.Download.BytesDownloaded = 700
	second := s.Snapshot()
	if second.Step == nil || second.Step.Index != 1 || second.Download == nil || second.Download.BytesDownloaded != 100 {
		t.Fatalf("second snapshot = %+v, want state isolated from returned pointers", second)
	}
}

func TestSubStepProgressClearingContract(t *testing.T) {
	s := New()
	if err := s.TransitionTo(Updating, "starting"); err != nil {
		t.Fatal(err)
	}
	s.UpdateSubStep(SubStepDownloading, "downloading")
	s.UpdateDownload(DownloadProgress{BytesDownloaded: 100, BytesTotal: 1000})
	if snap := s.Snapshot(); snap.Download == nil {
		t.Fatal("download progress missing during downloading sub-step")
	}

	s.UpdateSubStep(SubStepVerifying, "verifying")
	snap := s.Snapshot()
	if snap.SubStep != SubStepVerifying || snap.Detail != "verifying" || snap.Download != nil {
		t.Fatalf("state after leaving download = %+v", snap)
	}
}

func TestChangeHookReplacementContract(t *testing.T) {
	s := New()
	firstCalls := 0
	secondCalls := 0
	s.RegisterChangeHook(func(Snapshot) { firstCalls++ })
	s.RegisterChangeHook(func(Snapshot) { secondCalls++ })
	if s.GetChangeHook() == nil {
		t.Fatal("replacement hook = nil")
	}
	s.UpdateDetail("one")
	if firstCalls != 0 || secondCalls != 1 {
		t.Fatalf("hook calls = first:%d second:%d, want 0/1", firstCalls, secondCalls)
	}

	s.RegisterChangeHook(nil)
	if s.GetChangeHook() != nil {
		t.Fatal("hook was not cleared")
	}
	s.UpdateDetail("two")
	if secondCalls != 1 {
		t.Fatalf("cleared hook calls = %d, want 1", secondCalls)
	}
}

func TestCancellationResetOnTransitionContract(t *testing.T) {
	s := New()
	if err := s.TransitionTo(Updating, "starting"); err != nil {
		t.Fatal(err)
	}
	s.RequestCancel()
	if !s.CancelRequested() || !s.Snapshot().CancelRequested {
		t.Fatal("cancel request was not observable")
	}
	if err := s.TransitionTo(Cancelling, "cancelling"); err != nil {
		t.Fatal(err)
	}
	if s.CancelRequested() || s.Snapshot().CancelRequested {
		t.Fatal("phase transition did not clear cancellation state")
	}
}

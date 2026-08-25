package footprint

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// fakeDocker stubs the dockerops.Config methods RunOnce uses. We pass
// the real type here for signature compatibility — RunOnce calls only
// ListImagesByRepo / IsImageInUse / RemoveImage, all of which the test
// can control by injecting an exec function via DOCKER_BIN... easier:
// we test the algorithm directly via pruneRepo with a stub builder.
//
// Since RunOnce is the integration entry point and pruneRepo handles
// the bulk of the logic, we drive pruneRepo directly via a wrapper.

// imageAgeDays is pure — direct unit test
func TestImageAgeDays(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want int
	}{
		// Newer than now (negative-ish) → 0 days because of the inclusive
		// floor: anything within the same day reports 0.
		{"2026-05-08 11:00:00 +0000 UTC", 0},
		{"2026-05-07 12:00:00 +0000 UTC", 1},
		{"2026-05-01 12:00:00 +0000 UTC", 7},
		{"2026-04-08 12:00:00 +0000 UTC", 30},
		// Sentinel for parse error
		{"not-a-date", -1},
		{"", -1},
	}
	for _, c := range cases {
		got := imageAgeDays(c.in, now)
		if got != c.want {
			t.Errorf("imageAgeDays(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	outs := []PruneOutcome{
		{Action: "removed"},
		{Action: "removed"},
		{Action: "kept_hard"},
		{Action: "kept_keepN"},
		{Action: "kept_grace"},
		{Action: "kept_in_use"},
		{Action: "error"},
	}
	r, k, e := summarize(outs)
	if r != 2 || k != 4 || e != 1 {
		t.Errorf("summarize = (%d,%d,%d); want (2,4,1)", r, k, e)
	}
}

func TestRunOnceDisabled(t *testing.T) {
	ctx := context.Background()
	cfg := PruneConfig{Enabled: false, GraceDays: 7, KeepVersions: 3, Repos: []string{"clubfoundry"}}
	out, err := RunOnce(ctx, dockerops.Config{}, cfg)
	if err != nil {
		t.Fatalf("RunOnce disabled returned error: %v", err)
	}
	if len(out) != 1 || out[0].Action != "kept_grace" {
		t.Errorf("disabled outcome = %+v; want one kept_grace entry", out)
	}
}

func TestRunOnceInvalidConfig(t *testing.T) {
	ctx := context.Background()
	bad := []PruneConfig{
		{Enabled: true, GraceDays: 0, KeepVersions: 3},
		{Enabled: true, GraceDays: 7, KeepVersions: 0},
		{Enabled: true, GraceDays: -1, KeepVersions: 3},
	}
	for _, cfg := range bad {
		_, err := RunOnce(ctx, dockerops.Config{}, cfg)
		if err == nil {
			t.Errorf("RunOnce(%+v) should reject invalid config", cfg)
		}
	}
}

func TestWithAction(t *testing.T) {
	base := PruneOutcome{Time: time.Now(), Repo: "clubfoundry", Tag: "1.1.50", ID: "abc"}
	got := withAction(base, "removed", "test")
	if got.Action != "removed" || got.Reason != "test" {
		t.Errorf("withAction did not stamp action/reason: %+v", got)
	}
	if got.Repo != "clubfoundry" || got.Tag != "1.1.50" || got.ID != "abc" {
		t.Errorf("withAction lost base fields: %+v", got)
	}
}

func TestHardKeepTags(t *testing.T) {
	// All four hard-keep tags must be present + nothing else.
	if !hardKeepTags["current"] || !hardKeepTags["previous"] || !hardKeepTags["latest"] || !hardKeepTags["<none>"] {
		t.Error("hardKeepTags missing expected entry")
	}
	if hardKeepTags["1.1.50"] {
		t.Error("version tag should not be hard-keep")
	}
}

// helper to verify the algorithm via pruneRepo with a stub list.
// Builds an ImageInfo slice without actually shelling out.
func mockImages(repo string, n int, baseDay time.Time) []dockerops.ImageInfo {
	imgs := make([]dockerops.ImageInfo, n)
	for i := 0; i < n; i++ {
		imgs[i] = dockerops.ImageInfo{
			Repository: repo,
			Tag:        fmt.Sprintf("1.1.%d", 50-i),
			ID:         fmt.Sprintf("id-%d", i),
			CreatedAt:  baseDay.AddDate(0, 0, -i).Format("2006-01-02 15:04:05 -0700 MST"),
			Size:       "300MB",
			SizeBytes:  300_000_000,
		}
	}
	return imgs
}

func TestMockImagesShape(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	imgs := mockImages("clubfoundry", 5, now)
	if len(imgs) != 5 {
		t.Fatalf("len = %d, want 5", len(imgs))
	}
	if imgs[0].Tag != "1.1.50" || imgs[4].Tag != "1.1.46" {
		t.Errorf("tag ordering wrong: %v", []string{imgs[0].Tag, imgs[4].Tag})
	}
}

func TestBuildRepoReportCountsImageIDAliasesOnce(t *testing.T) {
	images := []dockerops.ImageInfo{
		{Repository: "clubfoundry", Tag: "1.4.11", ID: "sha256:current", CreatedAt: "2026-08-19 21:19:55 +0000 UTC", SizeBytes: 283_000_000},
		{Repository: "clubfoundry", Tag: "current", ID: "sha256:current", CreatedAt: "2026-08-19 21:19:55 +0000 UTC", SizeBytes: 283_000_000},
		{Repository: "clubfoundry", Tag: "1.4.9", ID: "sha256:previous", CreatedAt: "2026-08-16 21:32:46 +0000 UTC", SizeBytes: 283_000_000},
		{Repository: "clubfoundry", Tag: "previous", ID: "sha256:previous", CreatedAt: "2026-08-16 21:32:46 +0000 UTC", SizeBytes: 283_000_000},
	}

	report := buildRepoReport(images)
	if report.TotalBytes != 566_000_000 {
		t.Fatalf("total bytes = %d, want 566000000", report.TotalBytes)
	}
	if len(report.ImagesByTag) != 2 {
		t.Fatalf("unique images = %d, want 2", len(report.ImagesByTag))
	}
	if got := report.ImagesByTag[0].Tags; len(got) != 2 || got[0] != "1.4.11" || got[1] != "current" {
		t.Fatalf("current image tags = %v, want [1.4.11 current]", got)
	}
	if got := report.ImagesByTag[1].Tags; len(got) != 2 || got[0] != "1.4.9" || got[1] != "previous" {
		t.Fatalf("previous image tags = %v, want [1.4.9 previous]", got)
	}
}

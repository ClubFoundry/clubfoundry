// Package footprint reports disk use and prunes stale ClubFoundry image state.
package footprint

import (
	"context"
	"fmt"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// PruneConfig contains the operator settings used by one prune cycle.
type PruneConfig struct {
	Enabled       bool
	GraceDays     int
	KeepVersions  int
	Repos         []string
	Now           time.Time
	LogDir        string
	OperationMaxs time.Duration

	BuildCacheEnabled bool
	BuildCacheKeepGB  int
	BuildCacheAgeDays int
}

// PruneOutcome is one JSONL-compatible prune result.
type PruneOutcome struct {
	Time   time.Time `json:"time"`
	Repo   string    `json:"repo"`
	Tag    string    `json:"tag"`
	ID     string    `json:"id"`
	Action string    `json:"action"`
	Reason string    `json:"reason,omitempty"`
}

// RunOnce executes one synchronous image and build-cache prune cycle.
func RunOnce(ctx context.Context, dock dockerops.Config, cfg PruneConfig) ([]PruneOutcome, error) {
	if cfg.Now.IsZero() {
		cfg.Now = time.Now()
	}
	if !cfg.Enabled {
		return []PruneOutcome{{
			Time:   cfg.Now,
			Action: "kept_grace",
			Reason: "auto-prune disabled in settings",
		}}, nil
	}
	if cfg.GraceDays < 1 || cfg.KeepVersions < 1 {
		return nil, fmt.Errorf("invalid PruneConfig: GraceDays=%d KeepVersions=%d (both must be >=1)", cfg.GraceDays, cfg.KeepVersions)
	}

	var outcomes []PruneOutcome
	for _, repo := range cfg.Repos {
		repoOutcomes := pruneRepo(ctx, dock, repo, cfg)
		outcomes = append(outcomes, repoOutcomes...)
	}

	if cfg.BuildCacheEnabled {
		outcomes = append(outcomes, pruneBuildCache(ctx, dock, cfg)...)
	} else {
		outcomes = append(outcomes, PruneOutcome{
			Time:   cfg.Now,
			Repo:   "buildkit",
			Action: "buildcache_skipped",
			Reason: "build cache prune disabled in settings",
		})
	}

	if cfg.LogDir != "" {
		if err := writeJSONLLog(cfg.LogDir, cfg.Now, outcomes); err != nil {
			outcomes = append(outcomes, PruneOutcome{
				Time:   cfg.Now,
				Action: "error",
				Reason: fmt.Sprintf("log write failed: %v", err),
			})
		}
	}
	return outcomes, nil
}

package footprint

import (
	"context"
	"fmt"

	"github.com/clubfoundry/updater/internal/dockerops"
)

// Build cache uses a fixed one-hour inactivity threshold for compatibility.
const buildCacheUntilHours = 1

func pruneBuildCache(ctx context.Context, dock dockerops.Config, cfg PruneConfig) []PruneOutcome {
	if cfg.BuildCacheKeepGB < 1 {
		return []PruneOutcome{{
			Time:   cfg.Now,
			Repo:   "buildkit",
			Action: "error",
			Reason: fmt.Sprintf("invalid buildcache config: KeepGB=%d (must be >=1)", cfg.BuildCacheKeepGB),
		}}
	}
	keepBytes := int64(cfg.BuildCacheKeepGB) * 1024 * 1024 * 1024
	reclaimed, err := dock.BuildxPrune(ctx, keepBytes, buildCacheUntilHours)
	if err != nil {
		return []PruneOutcome{{
			Time:   cfg.Now,
			Repo:   "buildkit",
			Action: "error",
			Reason: fmt.Sprintf("buildx prune failed: %v", err),
		}}
	}
	return []PruneOutcome{{
		Time:   cfg.Now,
		Repo:   "buildkit",
		Action: "buildcache_pruned",
		Reason: fmt.Sprintf("reclaimed %d bytes (~%.2f GB), keep<=%dGB, until=%dh",
			reclaimed, float64(reclaimed)/(1<<30), cfg.BuildCacheKeepGB, buildCacheUntilHours),
	}}
}

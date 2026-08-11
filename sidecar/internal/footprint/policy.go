package footprint

import (
	"context"
	"fmt"
	"sort"

	"github.com/clubfoundry/updater/internal/dockerops"
)

var hardKeepTags = map[string]bool{
	"current":  true,
	"previous": true,
	"latest":   true,
	"<none>":   true,
}

func pruneRepo(ctx context.Context, dock dockerops.Config, repo string, cfg PruneConfig) []PruneOutcome {
	imgs, err := dock.ListImagesByRepo(ctx, repo)
	if err != nil {
		return []PruneOutcome{{
			Time:   cfg.Now,
			Repo:   repo,
			Action: "error",
			Reason: fmt.Sprintf("list images failed: %v", err),
		}}
	}
	if len(imgs) == 0 {
		return nil
	}

	// Docker's CreatedAt prefix sorts chronologically.
	sort.Slice(imgs, func(i, j int) bool { return imgs[i].CreatedAt > imgs[j].CreatedAt })

	versionedKept := 0
	out := make([]PruneOutcome, 0, len(imgs))
	for _, img := range imgs {
		base := PruneOutcome{Time: cfg.Now, Repo: repo, Tag: img.Tag, ID: img.ID}

		if hardKeepTags[img.Tag] {
			out = append(out, withAction(base, "kept_hard", "hard-keep tag (current/previous/latest/<none>)"))
			continue
		}

		if versionedKept < cfg.KeepVersions {
			versionedKept++
			out = append(out, withAction(base, "kept_keepN",
				fmt.Sprintf("kept %d/%d newest versioned", versionedKept, cfg.KeepVersions)))
			continue
		}

		ageDays := imageAgeDays(img.CreatedAt, cfg.Now)
		if ageDays >= 0 && ageDays < cfg.GraceDays {
			out = append(out, withAction(base, "kept_grace",
				fmt.Sprintf("age %d days < grace %d", ageDays, cfg.GraceDays)))
			continue
		}

		ref := repo + ":" + img.Tag
		if dock.IsImageInUse(ctx, ref) {
			out = append(out, withAction(base, "kept_in_use", "referenced by a container"))
			continue
		}

		if err := dock.RemoveImage(ctx, ref); err != nil {
			out = append(out, withAction(base, "error", err.Error()))
			continue
		}
		out = append(out, withAction(base, "removed",
			fmt.Sprintf("age %d days, beyond keep-%d", ageDays, cfg.KeepVersions)))
	}
	return out
}

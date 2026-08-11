package handlers

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
)

const updateDecisionTimeout = 5 * time.Second

// resolveUpdateDispatch is the decision layer for POST /update. It consults
// the cloud's updatePath, validates the caller-supplied target against it,
// and returns an updateDispatch describing what to run.
//
// Fallback behavior: if the cloud is unreachable / slow / returns a non-200,
// resolveUpdateDispatch returns {Target: requested} so the handler runs a
// single-hop update via registry fallback. Air-gapped installs therefore
// keep working — stepped-update + tarball-URL flows only activate when the
// cloud is actually responsive.
//
// Validation error (caller asked for a target not on the documented path)
// returns (updateDispatch{}, non-nil err).
func resolveUpdateDispatch(parent context.Context, d Deps, requested string) (updateDispatch, error) {
	if d.Cloud == nil || d.Cloud.BaseURL == "" {
		return updateDispatch{Target: requested}, nil
	}

	channel := "stable"
	if d.ConfigStore != nil {
		if set, _, err := d.ConfigStore.Load(); err == nil && set.Channel != "" {
			channel = set.Channel
		}
	}
	current := ""
	if d.Updater != nil {
		current = d.Updater.CurrentVersion(parent)
	}

	ctx, cancel := context.WithTimeout(parent, updateDecisionTimeout)
	defer cancel()
	resp, err := d.Cloud.CheckUpdates(ctx, current, channel)
	if err != nil || resp == nil {
		if err != nil {
			log.Printf("handlers: cloud lookup failed (%v); falling back to single-step registry pull", err)
		}
		return updateDispatch{Target: requested}, nil
	}

	// Published artifacts belong to the channel's latest version. Never attach
	// that URL and checksum to an explicit non-latest target.
	isLatestRequest := requested == "" || requested == resp.Latest
	mainPull := dockerops.PullOpts{}
	if isLatestRequest && resp.DownloadUrl != "" && resp.DownloadSha256 != "" {
		mainPull.URL = resp.DownloadUrl
		mainPull.Sha256 = resp.DownloadSha256
		// Use the advertised mirror set when available. Older responses keep
		// the single primary URL fallback.
		if len(resp.DownloadUrls) > 0 {
			mainPull.URLs = resp.DownloadUrls
		}
	}

	if resp.UpdatePath == nil {
		if !isLatestRequest {
			// Without a documented path, a non-latest target cannot be
			// matched to a verified artifact.
			return updateDispatch{}, fmt.Errorf(
				"target %q is not channel %q's current latest (%s) and the cloud advertises no update path for stepped install — switch to the channel where %s is the latest version, then retry without an explicit target",
				requested, channel, resp.Latest, requested,
			)
		}
		// No path info — single-hop to whatever `latest` currently is.
		return updateDispatch{Target: resp.Latest, MainPull: mainPull}, nil
	}

	path := resp.UpdatePath.Path
	if len(path) == 0 {
		if !isLatestRequest {
			return updateDispatch{}, fmt.Errorf(
				"target %q is not channel %q's current latest (%s) and the cloud returned an empty update path — switch to the channel where %s is the latest version, then retry without an explicit target",
				requested, channel, resp.Latest, requested,
			)
		}
		return updateDispatch{Target: resp.Latest, MainPull: mainPull}, nil
	}

	// Truncate the path at `requested` if the caller picked an intermediate
	// version. Empty `requested` = "the whole path, ending at path[-1]".
	if requested != "" && requested != path[len(path)-1] {
		idx := -1
		for i, v := range path {
			if v == requested {
				idx = i
				break
			}
		}
		if idx < 0 {
			return updateDispatch{}, fmt.Errorf(
				"target %q is not in the cloud-documented update path %v — operator must pick a version on the path or clear the target to use the default",
				requested, path,
			)
		}
		path = path[:idx+1]
	}

	// Stepped iff more than one hop OR cloud explicitly forbids direct jump
	// (the latter can fire on a 1-element path too — it means the single hop
	// must still run the guarded flow, but there's only one step to take).
	needsStepped := len(path) > 1 || !resp.UpdatePath.DirectJumpAllowed
	if needsStepped {
		// MainPull describes only the final channel artifact. Intermediate
		// hops continue to resolve through the registry.
		return updateDispatch{Path: path, Target: path[len(path)-1]}, nil
	}
	// A direct jump still needs the verified artifact published for Latest.
	// Intermediate tags are not guaranteed to exist across channels.
	if path[0] != resp.Latest {
		return updateDispatch{}, fmt.Errorf(
			"target %q is on channel %q's update path but is not its latest (%s); direct-jump-to-intermediate isn't supported because the cloud only ships per-channel-latest artifacts — clear the target to land on %s, or switch channel and retry",
			path[0], channel, resp.Latest, resp.Latest,
		)
	}
	return updateDispatch{Target: path[0], MainPull: mainPull}, nil
}

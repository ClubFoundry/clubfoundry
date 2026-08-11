package dockerops

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TrampolineOpts configures completion reporting and persistent output.
type TrampolineOpts struct {
	SentinelPath  string
	TrampolineID  string
	TargetVersion string
	OpID          string
	LogStdoutPath string
	LogStderrPath string
}

// SpawnRecreateTrampoline launches a short-lived helper that recreates sidecar.
func (c Config) SpawnRecreateTrampoline(ctx context.Context, service string, delaySec int, opts TrampolineOpts) error {
	if err := validateTrampolineRequest(service, opts); err != nil {
		return fmt.Errorf("invalid trampoline request: %w", err)
	}
	if delaySec <= 0 {
		delaySec = 5
	}

	info, err := c.inspectSelf(ctx)
	if err != nil {
		return fmt.Errorf("trampoline self-inspect: %w", err)
	}
	if info.image == "" {
		return fmt.Errorf("trampoline: could not determine self image")
	}
	if info.composeHostDir == "" {
		return fmt.Errorf("trampoline: could not determine compose dir host path (no %q mount)", c.ComposeDir)
	}
	if err := validateTrampolinePath("compose host directory", info.composeHostDir); err != nil {
		return fmt.Errorf("invalid trampoline self-inspect result: %w", err)
	}

	args := []string{
		"run", "--rm", "-d",
		"--network", "host",
		"--name", fmt.Sprintf("%s-trampoline-%d", service, time.Now().Unix()),
	}
	hasComposeHostDirMount := false
	for _, mount := range info.mounts {
		mode := "rw"
		if !mount.rw {
			mode = "ro"
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", mount.source, mount.destination, mode))
		if mount.destination == info.composeHostDir {
			hasComposeHostDirMount = true
		}
	}
	if !hasComposeHostDirMount {
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", info.composeHostDir, info.composeHostDir))
	}
	args = append(args,
		"--entrypoint", "sh",
		info.image,
		"-c", buildTrampolineShell(info.composeHostDir, service, delaySec, opts),
	)

	out, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("trampoline spawn (image=%s, compose=%s): %w: %s", info.image, info.composeHostDir, err, out)
	}
	c.logf("self-update trampoline spawned: %s, will recreate %s in %ds (replicated %d mounts, sentinel=%q)",
		strings.TrimSpace(string(out)), service, delaySec, len(info.mounts), opts.SentinelPath)
	return nil
}

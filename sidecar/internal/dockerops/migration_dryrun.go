package dockerops

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DryrunOpts describes the disposable database copy used for migration checks.
type DryrunOpts struct {
	HostDataDir  string
	SourceDBFile string
	CopyDBFile   string
	Timeout      time.Duration
}

// RunMigrationDryrun runs candidate migrations against a disposable copy.
func (c Config) RunMigrationDryrun(ctx context.Context, imageRef string, opts DryrunOpts) error {
	if imageRef == "" {
		return fmt.Errorf("RunMigrationDryrun: empty imageRef")
	}
	if opts.HostDataDir == "" {
		return fmt.Errorf("RunMigrationDryrun: empty HostDataDir")
	}
	if opts.SourceDBFile == "" {
		opts.SourceDBFile = "clm.db"
	}
	if opts.CopyDBFile == "" {
		opts.CopyDBFile = fmt.Sprintf("clm.db.dryrun-%d", time.Now().Unix())
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"run", "--rm",
		"--network", "host",
		"-v", fmt.Sprintf("%s:/data", opts.HostDataDir),
		"--entrypoint", "node",
		imageRef,
		"/app/dist/db/migrate-dryrun.js",
		"--source=/data/" + opts.SourceDBFile,
		"--copy-to=/data/" + opts.CopyDBFile,
	}
	out, err := c.run(runCtx, args...)
	if err != nil {
		return fmt.Errorf("schema dry-run: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

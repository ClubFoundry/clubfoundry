package updater

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/clubfoundry/updater/internal/dockerops"
	"github.com/clubfoundry/updater/internal/state"
)

// runSchemaDryrun validates candidate migrations against a copy of the live
// database before Stop. The opt-in gate requires both the enable flag and the
// host data path; an executed migration failure blocks the update.
func (d *Deps) runSchemaDryrun(ctx context.Context, toVersion string, logW io.Writer) error {
	if os.Getenv("CLM_SCHEMA_DRYRUN_ENABLED") != "true" {
		fmt.Fprintf(logW, "[1.5/5] schema dry-run: skipped (CLM_SCHEMA_DRYRUN_ENABLED != \"true\")\n")
		return nil
	}
	hostDataDir := os.Getenv("CLM_HOST_DATA_DIR")
	if hostDataDir == "" {
		fmt.Fprintf(logW, "[1.5/5] schema dry-run: skipped (CLM_HOST_DATA_DIR not set)\n")
		return nil
	}
	if toVersion == "" || toVersion == "latest" || toVersion == "unknown" {
		fmt.Fprintf(logW, "[1.5/5] schema dry-run: skipped (toVersion=%q is unresolved)\n", toVersion)
		return nil
	}
	d.State.UpdateSubStep(state.SubStepVerifying, "Schema dry-run on copy of database")
	imageRef := fmt.Sprintf("%s:%s", d.Docker.MainServiceName(), toVersion)
	fmt.Fprintf(logW, "[1.5/5] schema dry-run: image=%s host_data_dir=%s\n", imageRef, hostDataDir)
	err := d.Docker.RunMigrationDryrun(ctx, imageRef, dockerops.DryrunOpts{
		HostDataDir:  hostDataDir,
		SourceDBFile: "clm.db",
		CopyDBFile:   fmt.Sprintf("clm.db.dryrun-%d", time.Now().Unix()),
	})
	if err != nil {
		fmt.Fprintf(logW, "      schema dry-run FAILED: %v\n", err)
		return err
	}
	fmt.Fprintf(logW, "      schema dry-run OK\n")
	return nil
}

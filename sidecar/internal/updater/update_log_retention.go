package updater

import (
	"time"
)

const (
	retainSuccessLogs    = 5
	retainFailureBundles = 10
	retainTotalBytesCap  = int64(200 * 1024 * 1024)
	retainMaxAge         = 90 * 24 * time.Hour
)

// SweepDiagnosticRetention applies age, count, and combined-size limits to
// successful operation logs and archived failure bundles.
func SweepDiagnosticRetention(dataDir string) {
	if dataDir == "" {
		return
	}
	pruneByAge(updateLogsDir(dataDir), retainMaxAge)
	pruneByAge(updateFailuresDir(dataDir), retainMaxAge)
	pruneByCount(updateLogsDir(dataDir), retainSuccessLogs)
	pruneByCount(updateFailuresDir(dataDir), retainFailureBundles)
	pruneByBytes(dataDir, retainTotalBytesCap)
}

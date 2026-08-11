package handlers

import (
	"archive/zip"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

// handleDiagnosticBundle streams a best-effort point-in-time support archive.
func handleDiagnosticBundle(dataDir, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		if dataDir == "" {
			writeError(w, http.StatusServiceUnavailable, "no data dir configured")
			return
		}
		filename := fmt.Sprintf("clubfoundry-diag-%s-%s.zip",
			version, time.Now().UTC().Format("20060102T150405Z"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename=%q`, filename))
		w.Header().Set("Cache-Control", "no-store")

		zw := zip.NewWriter(w)
		defer zw.Close()

		writeBundleInfo(zw, dataDir, version)
		writeBundleStateFiles(zw, dataDir)
		writeBundleSentinels(zw, dataDir)
		writeBundleDirTree(zw, filepath.Join(dataDir, "update-logs"), "update-logs")
		writeBundleDirTree(zw, filepath.Join(dataDir, "update-failures"), "update-failures")
		writeBundleComposeFile(zw, dataDir)
	}
}

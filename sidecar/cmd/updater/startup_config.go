package main

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/clubfoundry/updater/internal/backup"
	"github.com/clubfoundry/updater/internal/cloud"
	"github.com/clubfoundry/updater/internal/health"
	"github.com/clubfoundry/updater/internal/telemetry"
)

type runtimeSettings struct {
	Addr    string
	DataDir string
}

func loadRuntimeSettings() runtimeSettings {
	settings := runtimeSettings{
		Addr:    os.Getenv("CLUBFOUNDRY_UPDATER_ADDR"),
		DataDir: os.Getenv("CLUBFOUNDRY_DATA_DIR"),
	}
	if settings.Addr == "" {
		settings.Addr = ":3001"
	}
	if settings.DataDir == "" {
		settings.DataDir = "/app/data"
	}
	return settings
}

func backupConfigForDataDir(dataDir string) backup.Config {
	backupCfg := backup.DefaultConfig()
	if dataDir != "/app/data" {
		backupCfg.DBPath = filepath.Join(dataDir, "clm.db")
		backupCfg.BackupsDir = filepath.Join(dataDir, "backups")
	}
	return backupCfg
}

func telemetryForCloud(cloudClient *cloud.Client, healthCheck *health.Checker, dataDir string) *telemetry.Reporter {
	if cloudClient == nil || cloudClient.BaseURL == "" {
		return nil
	}
	return &telemetry.Reporter{
		CloudBaseURL: cloudClient.BaseURL,
		InstanceID:   os.Getenv("CLUBFOUNDRY_INSTANCE_ID"),
		Health:       healthCheck,
		LogDir:       filepath.Join(dataDir, "update-logs"),
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

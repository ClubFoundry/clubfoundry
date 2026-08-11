package backup

import "os"

// Config describes the live database and backup retention layout.
type Config struct {
	DBPath     string
	BackupsDir string
	KeepN      int
}

// DefaultConfig returns the standard paths and retention limit.
func DefaultConfig() Config {
	return Config{
		DBPath:     envOr("CLUBFOUNDRY_DB_PATH", "/app/data/clm.db"),
		BackupsDir: envOr("CLUBFOUNDRY_BACKUPS_DIR", "/app/data/backups"),
		KeepN:      5,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

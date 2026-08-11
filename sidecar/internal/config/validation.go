package config

import (
	"fmt"
	"regexp"
)

var windowRe = regexp.MustCompile(`^([0-1][0-9]|2[0-3]):[0-5][0-9]-([0-1][0-9]|2[0-3]):[0-5][0-9]$`)

// Validate returns an error wrapping ErrValidation for an invalid field.
func Validate(s Settings) error {
	if s.Channel != "alpha" && s.Channel != "beta" && s.Channel != "stable" && s.Channel != "lts" {
		return fmt.Errorf("%w: channel must be alpha, beta, stable, or lts, got %q", ErrValidation, s.Channel)
	}
	if !windowRe.MatchString(s.UpdateWindow) {
		return fmt.Errorf("%w: update_window must be HH:MM-HH:MM, got %q", ErrValidation, s.UpdateWindow)
	}
	// Clamp to sane bounds: below 5 minutes hammers the cloud, while above
	// one day leaves security fixes unapplied too long.
	if s.CheckIntervalSec < 300 || s.CheckIntervalSec > 86400 {
		return fmt.Errorf("%w: check_interval_sec must be 300..86400, got %d", ErrValidation, s.CheckIntervalSec)
	}
	if s.AutoPruneGraceDays < 1 || s.AutoPruneGraceDays > 30 {
		return fmt.Errorf("%w: auto_prune_grace_days must be 1..30, got %d", ErrValidation, s.AutoPruneGraceDays)
	}
	if s.AutoPruneKeepVersions < 1 || s.AutoPruneKeepVersions > 10 {
		return fmt.Errorf("%w: auto_prune_keep_versions must be 1..10, got %d", ErrValidation, s.AutoPruneKeepVersions)
	}
	if s.AutoPruneBuildCacheKeepGB < 1 || s.AutoPruneBuildCacheKeepGB > 200 {
		return fmt.Errorf("%w: auto_prune_buildcache_keep_gb must be 1..200, got %d", ErrValidation, s.AutoPruneBuildCacheKeepGB)
	}
	if s.AutoPruneBuildCacheAgeDays < 1 || s.AutoPruneBuildCacheAgeDays > 30 {
		return fmt.Errorf("%w: auto_prune_buildcache_age_days must be 1..30, got %d", ErrValidation, s.AutoPruneBuildCacheAgeDays)
	}
	return nil
}

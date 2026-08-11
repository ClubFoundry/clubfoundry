package config

// Defaults returns the built-in default values.
//
// AutoUpdate defaults on for the stable channel. The frontend disables it when
// switching into alpha or beta and requires an explicit, visible transition
// when enabling it again for stable or LTS.
func Defaults() Settings {
	return Settings{
		AutoUpdate:       true,
		UpdateWindow:     "03:00-05:00",
		Channel:          "stable",
		CheckIntervalSec: 3600,
		// AutoPruneOptOut intentionally false (opt-out semantics keeps
		// missing-field = enabled).
		AutoPruneOptOut: false,
		// The backend enforces a one-day grace period. Current, previous, and
		// keep-N retention provide the remaining rollback coverage.
		AutoPruneGraceDays:    1,
		AutoPruneKeepVersions: 2,

		AutoPruneBuildCacheOptOut:  false,
		AutoPruneBuildCacheKeepGB:  2,
		AutoPruneBuildCacheAgeDays: 3,
	}
}

// merge fills in zero-valued fields from defaults. This keeps older partial
// config files from silently disabling fields added in later versions.
func merge(in Settings) Settings {
	d := Defaults()
	if in.UpdateWindow == "" {
		in.UpdateWindow = d.UpdateWindow
	}
	if in.Channel == "" {
		in.Channel = d.Channel
	}
	if in.CheckIntervalSec <= 0 {
		in.CheckIntervalSec = d.CheckIntervalSec
	}
	if in.AutoPruneGraceDays <= 0 {
		in.AutoPruneGraceDays = d.AutoPruneGraceDays
	}
	if in.AutoPruneKeepVersions <= 0 {
		in.AutoPruneKeepVersions = d.AutoPruneKeepVersions
	}
	if in.AutoPruneBuildCacheKeepGB <= 0 {
		in.AutoPruneBuildCacheKeepGB = d.AutoPruneBuildCacheKeepGB
	}
	if in.AutoPruneBuildCacheAgeDays <= 0 {
		in.AutoPruneBuildCacheAgeDays = d.AutoPruneBuildCacheAgeDays
	}
	return in
}

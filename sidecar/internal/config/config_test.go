package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultsOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "not-yet.json"))
	got, loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded {
		t.Errorf("expected loaded=false for missing file")
	}
	if got != Defaults() {
		t.Errorf("expected defaults, got %+v", got)
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "updater.json"))

	in := Settings{
		AutoUpdate:                 true,
		UpdateWindow:               "04:00-06:00",
		Channel:                    "beta",
		CheckIntervalSec:           1800,
		AutoPruneOptOut:            true,
		AutoPruneGraceDays:         7,
		AutoPruneKeepVersions:      3,
		AutoPruneBuildCacheKeepGB:  5,
		AutoPruneBuildCacheAgeDays: 3,
	}
	if err := s.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded || got != in {
		t.Fatalf("roundtrip mismatch: loaded=%v got=%+v", loaded, got)
	}
}

func TestLoadMergesPartialLegacyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.json")
	if err := os.WriteFile(path, []byte(`{"auto_update":false,"channel":"beta"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, loaded, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded {
		t.Fatal("existing partial config must report loaded=true")
	}
	if got.AutoUpdate || got.Channel != "beta" {
		t.Fatalf("explicit legacy values were not preserved: %+v", got)
	}
	defaults := Defaults()
	if got.UpdateWindow != defaults.UpdateWindow ||
		got.CheckIntervalSec != defaults.CheckIntervalSec ||
		got.AutoPruneGraceDays != defaults.AutoPruneGraceDays ||
		got.AutoPruneKeepVersions != defaults.AutoPruneKeepVersions ||
		got.AutoPruneBuildCacheKeepGB != defaults.AutoPruneBuildCacheKeepGB ||
		got.AutoPruneBuildCacheAgeDays != defaults.AutoPruneBuildCacheAgeDays {
		t.Fatalf("missing fields were not merged from defaults: %+v", got)
	}
}

func TestLoadMalformedConfigReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, loaded, err := NewStore(path).Load()
	if err == nil || loaded || got != Defaults() {
		t.Fatalf("result = (%+v, %v, %v), want (defaults, false, parse error)", got, loaded, err)
	}
}

func TestSaveValidationAndAtomicFileContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "updater.json")
	store := NewStore(path)
	invalid := Defaults()
	invalid.Channel = "nightly"
	if err := store.Save(invalid); !errors.Is(err, ErrValidation) {
		t.Fatalf("Save validation error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid config created final file: %v", err)
	}

	if err := store.Save(Defaults()); err != nil {
		t.Fatalf("Save defaults: %v", err)
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains after atomic rename: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %o, want 644", info.Mode().Perm())
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		s    Settings
		ok   bool
	}{
		{"defaults ok", Defaults(), true},
		{"alpha channel ok", Settings{Channel: "alpha", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, true},
		{"beta channel ok", Settings{Channel: "beta", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, true},
		{"lts channel ok", Settings{Channel: "lts", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, true},
		{"bad channel", Settings{Channel: "nightly", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		{"bad window", Settings{Channel: "stable", UpdateWindow: "25:00-99:99", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		{"too-fast check", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 60, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		{"too-slow check", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 999999, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		// Image-prune validation.
		{"prune grace too low", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 0, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		{"prune grace too high", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 31, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		{"prune keep too low", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 0, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		{"prune keep too high", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 11, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 3}, false},
		// Build-cache prune validation.
		{"buildcache keep gb too low", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 0, AutoPruneBuildCacheAgeDays: 3}, false},
		{"buildcache keep gb too high", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 201, AutoPruneBuildCacheAgeDays: 3}, false},
		{"buildcache age too low", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 0}, false},
		{"buildcache age too high", Settings{Channel: "stable", UpdateWindow: "03:00-04:00", CheckIntervalSec: 3600, AutoPruneGraceDays: 7, AutoPruneKeepVersions: 3, AutoPruneBuildCacheKeepGB: 5, AutoPruneBuildCacheAgeDays: 31}, false},
	}
	for _, c := range cases {
		err := Validate(c.s)
		if c.ok && err != nil {
			t.Errorf("%s: expected ok, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

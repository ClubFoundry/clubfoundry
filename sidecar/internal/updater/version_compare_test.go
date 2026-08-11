package updater

import "testing"

func TestIsStrictlyNewerSidecarVersion(t *testing.T) {
	cases := []struct {
		name       string
		current    string
		advertised string
		want       bool
	}{
		// Happy path — letter rollover within MAJOR=1.
		{"single letter newer", "v1.K", "v1.L", true},
		{"single letter older", "v1.L", "v1.K", false},
		{"equal version", "v1.T", "v1.T", false},
		// Multi-letter (post-Z rollover).
		{"AA newer than Z", "v1.Z", "v1.AA", true},
		{"Z older than AA", "v1.AA", "v1.Z", false},
		{"AB newer than AA", "v1.AA", "v1.AB", true},
		// Major bump.
		{"major bump", "v1.Z", "v2.A", true},
		{"major regression", "v2.A", "v1.Z", false},
		// Unparseable versions fail closed so stale metadata cannot trigger a downgrade.
		{"empty current", "", "v1.L", false},
		{"empty advertised", "v1.L", "", false},
		{"both empty", "", "", false},
		// `current=="dev"` is the corrupted-binary marker. To break the
		// self-heal catch-22 (a binary that can't recognize any version as
		// newer than its own "dev" string is stuck on the broken image),
		// "dev" treats any WELL-FORMED advertised version as newer.
		// Malformed advertised still returns false — we don't license
		// upgrades to garbage just because we're broken.
		{"dev → well-formed = upgrade", "dev", "v1.L", true},
		{"dev → multi-letter = upgrade", "dev", "v1.AA", true},
		{"dev → empty = no upgrade", "dev", "", false},
		{"dev → garbage = no upgrade", "dev", "garbage", false},
		{"dev → dev = no upgrade", "dev", "dev", false},
		{"non-v prefix", "1.L", "v1.L", false},
		{"missing dot", "v1L", "v1.L", false},
		{"non-letter chars", "v1.K", "v1.L1", false},
		{"lowercase letters", "v1.K", "v1.l", false},
		{"leading zero major", "v01.A", "v1.A", false},
		// A lower advertised train must not produce an update offer.
		{"published-train downgrade", "v1.T", "v1.L", false},
		// Switching to a channel with an older single-letter train must remain
		// a no-op. Multi-letter > single-letter means
		// IsStrictlyNewerSidecarVersion("v1.II", "v1.X") = false; this
		// is correct for the BACKGROUND POLLER (which shouldn't silently
		// downgrade across publish-train races), but caused the operator's
		// explicit /self-update click to 409 with "refusing downgrade".
		// The handler-level fix (handlers.go /self-update no longer
		// applies this guard on operator-explicit paths) leaves this
		// function intact — its return value here remains the right
		// answer for the automatic polling context.
		{"channel-switch downgrade alpha→beta — auto-poll context", "v1.II", "v1.X", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsStrictlyNewerSidecarVersion(c.current, c.advertised); got != c.want {
				t.Errorf("IsStrictlyNewerSidecarVersion(%q, %q) = %v, want %v",
					c.current, c.advertised, got, c.want)
			}
		})
	}
}

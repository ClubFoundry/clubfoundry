package dockerops

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildTrampolineShellLegacyContract(t *testing.T) {
	script := buildTrampolineShell("/compose dir", "clubfoundry-updater", 7, TrampolineOpts{})

	want := `echo "[trampoline] start id= target= service=clubfoundry-updater sleep=7s wall=$(date -u +%Y-%m-%dT%H:%M:%SZ)"; sleep 7 && cd "/compose dir" && docker rm -f clubfoundry-updater >/dev/null 2>&1 || true; docker compose up -d --force-recreate clubfoundry-updater 2>&1`
	if script != want {
		t.Fatalf("unexpected legacy trampoline script:\n got: %s\nwant: %s", script, want)
	}
	if strings.Contains(script, "--remove-orphans") {
		t.Fatal("legacy trampoline contract unexpectedly enables --remove-orphans")
	}
}

func TestBuildTrampolineShellSentinelContract(t *testing.T) {
	opts := TrampolineOpts{
		SentinelPath:  "data/sentinels/update.json",
		TrampolineID:  "trampoline-123",
		TargetVersion: "1.2.3-alpha.1",
		OpID:          "operation-456",
		LogStdoutPath: "data/logs/trampoline.stdout",
		LogStderrPath: "data/logs/trampoline.stderr",
	}
	script := buildTrampolineShell("/compose dir", "clubfoundry-updater", 5, opts)

	required := []string{
		`exec >"data/logs/trampoline.stdout" 2>"data/logs/trampoline.stderr"; `,
		`if ! cd "/compose dir"; then`,
		`"exit_code":99`,
		`"error":"cd_to_compose_host_dir_failed"`,
		`docker compose up -d --force-recreate --remove-orphans clubfoundry-updater || rc=$?`,
		`"trampoline_id":"trampoline-123"`,
		`"target_version":"1.2.3-alpha.1"`,
		`"service":"clubfoundry-updater"`,
		`"op_id":"operation-456"`,
		`> "data/sentinels/update.json.tmp" && mv "data/sentinels/update.json.tmp" "data/sentinels/update.json"`,
		`exit $rc`,
	}
	for _, fragment := range required {
		if !strings.Contains(script, fragment) {
			t.Errorf("trampoline script is missing %q", fragment)
		}
	}

	cdIndex := strings.Index(script, `if ! cd "/compose dir"; then`)
	removeIndex := strings.Index(script, "docker rm -f clubfoundry-updater")
	if cdIndex < 0 || removeIndex < 0 || cdIndex >= removeIndex {
		t.Fatalf("compose directory guard must run before container removal: %s", script)
	}
}

func TestBuildTrampolineShellRequiresBothLogPaths(t *testing.T) {
	script := buildTrampolineShell("/compose", "clubfoundry-updater", 5, TrampolineOpts{
		LogStdoutPath: "data/logs/trampoline.stdout",
	})

	if strings.HasPrefix(script, "exec >") {
		t.Fatal("trampoline must not redirect output when only one log path is configured")
	}
}

func TestValidateTrampolineRequestContract(t *testing.T) {
	valid := TrampolineOpts{
		SentinelPath:  "/app/data/update sentinels/result.json",
		TrampolineID:  "trampoline-123",
		TargetVersion: "1.2.3-alpha.1+build",
		OpID:          "operation:456",
		LogStdoutPath: "/app/data/update logs/stdout.log",
		LogStderrPath: "/app/data/update logs/stderr.log",
	}
	if err := validateTrampolineRequest("clubfoundry-updater", valid); err != nil {
		t.Fatalf("valid trampoline request rejected: %v", err)
	}

	tests := []struct {
		name    string
		service string
		opts    TrampolineOpts
	}{
		{name: "empty service"},
		{name: "service command", service: "updater;touch", opts: valid},
		{name: "metadata quote", service: "updater", opts: withTargetVersion(valid, "1.2.3'bad")},
		{name: "path substitution", service: "updater", opts: withSentinelPath(valid, "/app/data/$(touch bad)")},
		{name: "path newline", service: "updater", opts: withStdoutPath(valid, "/app/data/log\nname")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateTrampolineRequest(tt.service, tt.opts); err == nil {
				t.Fatal("expected unsafe trampoline request to fail")
			}
		})
	}
}

func TestValidateTrampolinePathContract(t *testing.T) {
	if err := validateTrampolinePath("compose host directory", "/mnt/pool/club foundry"); err != nil {
		t.Fatalf("valid compose path rejected: %v", err)
	}
	if err := validateTrampolinePath("compose host directory", "/mnt/$(touch bad)"); err == nil {
		t.Fatal("expected shell substitution in compose path to fail")
	}
}

func withTargetVersion(opts TrampolineOpts, value string) TrampolineOpts {
	opts.TargetVersion = value
	return opts
}

func withSentinelPath(opts TrampolineOpts, value string) TrampolineOpts {
	opts.SentinelPath = value
	return opts
}

func withStdoutPath(opts TrampolineOpts, value string) TrampolineOpts {
	opts.LogStdoutPath = value
	return opts
}

func TestParseSelfInspectDirectoryMountContract(t *testing.T) {
	raw := "ghcr.io/clubfoundry/updater:1.2.3|" +
		"/app|/mnt/pool/clubfoundry|true;" +
		"/app/data|/mnt/pool/clubfoundry/data|false;" +
		"malformed;"

	got, err := parseSelfInspect(raw, "/app")
	if err != nil {
		t.Fatalf("parse self inspect: %v", err)
	}
	want := selfInspect{
		image:          "ghcr.io/clubfoundry/updater:1.2.3",
		composeHostDir: "/mnt/pool/clubfoundry",
		mounts: []selfMount{
			{source: "/mnt/pool/clubfoundry", destination: "/app", rw: true},
			{source: "/mnt/pool/clubfoundry/data", destination: "/app/data", rw: false},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected self inspect result:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseSelfInspectSingleFileFallbackContract(t *testing.T) {
	raw := "updater:image|/app/docker-compose.yml|/mnt/pool/project/docker-compose.yml|true;"

	got, err := parseSelfInspect(raw, "")
	if err != nil {
		t.Fatalf("parse self inspect: %v", err)
	}
	if got.composeHostDir != "/mnt/pool/project" {
		t.Fatalf("unexpected compose host directory: %q", got.composeHostDir)
	}
}

func TestParseSelfInspectPrefersDirectoryMountContract(t *testing.T) {
	raw := "updater:image|" +
		"/app/docker-compose.yml|/mnt/fallback/docker-compose.yml|true;" +
		"/app|/mnt/preferred|true;"

	got, err := parseSelfInspect(raw, "/app")
	if err != nil {
		t.Fatalf("parse self inspect: %v", err)
	}
	if got.composeHostDir != "/mnt/preferred" {
		t.Fatalf("directory mount must win over file fallback, got %q", got.composeHostDir)
	}
}

func TestParseSelfInspectRejectsUnexpectedOutput(t *testing.T) {
	if _, err := parseSelfInspect("image-without-mount-separator", "/app"); err == nil {
		t.Fatal("expected malformed docker inspect output to fail")
	}
}

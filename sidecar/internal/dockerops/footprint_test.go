package dockerops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSizeString(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0B", 0},
		{"N/A", 0},
		{"144MB", 144_000_000},
		{"398MB", 398_000_000},
		{"5.281GB", 5_281_000_000},
		{"1.29GB", 1_290_000_000},
		{"29.96kB", 29_960},
		{"1B", 1},
		{"1024B", 1024},
		{"1TB", 1_000_000_000_000},
		{"  398MB  ", 398_000_000},
		{"398 MB", 398_000_000}, // tolerate space between number and unit
		{"PB-not-supported", 0},
		{"abc", 0},
	}
	for _, c := range cases {
		got := parseSizeString(c.in)
		if got != c.want {
			t.Errorf("parseSizeString(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFindDFEntry(t *testing.T) {
	entries := []SystemDFEntry{
		{Type: "Images", SizeBytes: 5_281_000_000, ReclaimableBytes: 2_733_000_000},
		{Type: "Containers", SizeBytes: 0, ReclaimableBytes: 0},
		{Type: "Local Volumes", SizeBytes: 0, ReclaimableBytes: 0},
		{Type: "Build Cache", SizeBytes: 3_337_000_000, ReclaimableBytes: 1_723_000_000},
	}
	got := FindDFEntry(entries, "Images")
	if got.SizeBytes != 5_281_000_000 {
		t.Errorf("FindDFEntry(Images).SizeBytes = %d, want 5281000000", got.SizeBytes)
	}
	missing := FindDFEntry(entries, "Nope")
	if missing.SizeBytes != 0 || missing.Type != "" {
		t.Errorf("FindDFEntry(missing) = %+v, want zero", missing)
	}
}

func TestParseReclaimedBytes(t *testing.T) {
	for input, want := range map[string]int64{
		"Total reclaimed space: 31.09GB": 33_382_633_308,
		"Total reclaimed: 2MB":           2 * 1024 * 1024,
		"Total reclaimed space: 1.5TB":   1_649_267_441_664,
		"no trailer":                     0,
	} {
		if got := parseReclaimedBytes(input); got != want {
			t.Errorf("parseReclaimedBytes(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestFootprintDockerCommandContracts(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	t.Setenv("FAKE_DOCKER_ARGS_FILE", argsPath)
	cfg := Config{DockerBin: writeFakeDocker(t), ComposeDir: dir, DefaultTimeout: time.Second}

	t.Run("images", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", `{"Repository":"clubfoundry","Tag":"1.3.138","ID":"sha256:a","Size":"398MB"}`)
		t.Setenv("FAKE_DOCKER_STDOUT_2", `{"Repository":"clubfoundry-updater","Tag":"v3.AH","ID":"sha256:b","Size":"100MB"}`)
		images, err := cfg.ListImagesByRepo(context.Background(), "clubfoundry")
		if err != nil || len(images) != 1 || images[0].SizeBytes != 398_000_000 {
			t.Fatalf("ListImagesByRepo = (%+v, %v)", images, err)
		}
	})

	t.Run("system df", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", `{"Type":"Images","Size":"5.281GB","Reclaimable":"2.733GB"}`)
		t.Setenv("FAKE_DOCKER_STDOUT_2", "")
		entries, err := cfg.SystemDF(context.Background())
		if err != nil || len(entries) != 1 || entries[0].SizeBytes != 5_281_000_000 || entries[0].ReclaimableBytes != 2_733_000_000 {
			t.Fatalf("SystemDF = (%+v, %v)", entries, err)
		}
	})

	t.Run("buildx prune", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", "Total reclaimed space: 1.5GB")
		reclaimed, err := cfg.BuildxPrune(context.Background(), 2_000_000_000, 72)
		if err != nil || reclaimed != 1_610_612_736 {
			t.Fatalf("BuildxPrune = (%d, %v)", reclaimed, err)
		}
	})

	t.Run("image in use", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", "container-id")
		if !cfg.IsImageInUse(context.Background(), "clubfoundry:1.3.138") {
			t.Fatal("container reference must report in-use")
		}
		t.Setenv("FAKE_DOCKER_STDOUT", "")
		if cfg.IsImageInUse(context.Background(), "clubfoundry:old") {
			t.Fatal("empty container list must report unused")
		}
		t.Setenv("FAKE_DOCKER_EXIT", "1")
		if !cfg.IsImageInUse(context.Background(), "clubfoundry:unknown") {
			t.Fatal("Docker error must fail safe as in-use")
		}
		t.Setenv("FAKE_DOCKER_EXIT", "")
	})

	if err := cfg.RemoveImage(context.Background(), "clubfoundry:old"); err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}
	if err := cfg.RemoveImage(context.Background(), ""); err == nil {
		t.Fatal("empty image ref must be rejected")
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	normalizedArgs := strings.ReplaceAll(string(args), `"`, "")
	for _, fragment := range []string{
		"images --format {{json .}} clubfoundry",
		"system df --format {{json .}}",
		"buildx prune --force --keep-storage 2000000000 --filter until=72h",
		"ps -a --filter ancestor=clubfoundry:1.3.138 --format {{.ID}}",
		"rmi clubfoundry:old",
	} {
		if !strings.Contains(normalizedArgs, fragment) {
			t.Errorf("command log does not contain %q:\n%s", fragment, args)
		}
	}
}

func TestListImagesRejectsEmptyRepo(t *testing.T) {
	_, err := (Config{}).ListImagesByRepo(context.Background(), "")
	if err == nil || err.Error() != "ListImagesByRepo: empty repo" {
		t.Fatalf("error = %v", err)
	}
}

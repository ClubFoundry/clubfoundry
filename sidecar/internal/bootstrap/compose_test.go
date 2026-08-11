package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteComposeFile_AllPlaceholdersResolved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")

	err := WriteComposeFile(path, ComposeParams{
		MainImage:       "clubfoundry:1.2.3",
		MainService:     "clubfoundry",
		UpdaterImage:    "clubfoundry-updater:v1.G",
		UpdaterService:  "clubfoundry-updater",
		HostDataDir:     "/opt/clubfoundry/data",
		HostComposeFile: "/opt/clubfoundry/docker-compose.yml",
		CloudURL:        "https://clubfoundry.net",
	})
	if err != nil {
		t.Fatalf("WriteComposeFile: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := string(body)

	// No `{{...}}` placeholders should survive — a template field added
	// to params but not interpolated would silently render as literal
	// braces in production.
	if strings.Contains(out, "{{") {
		t.Errorf("template still contains unresolved placeholder: %s", out)
	}

	must := []string{
		"image: clubfoundry:1.2.3",
		"image: clubfoundry-updater:v1.G",
		"container_name: clubfoundry",
		"container_name: clubfoundry-updater",
		"network_mode: host",
		"- /opt/clubfoundry/data:/app/data",
		"- /opt/clubfoundry/docker-compose.yml:/app/docker-compose.yml",
		"CLUBFOUNDRY_CLOUD_URL: https://clubfoundry.net",
		"CLUBFOUNDRY_UPDATER_ADDR: 127.0.0.1:3001",
		"env_file: ./data/.env",
	}
	for _, want := range must {
		if !strings.Contains(out, want) {
			t.Errorf("missing required line %q in:\n%s", want, out)
		}
	}
}

func TestWriteComposeFile_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("# operator-supplied"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteComposeFile(path, ComposeParams{
		MainImage:    "clubfoundry:1.0.0",
		MainService:  "clubfoundry",
		UpdaterImage: "clubfoundry-updater:dev",
	})
	if err == nil {
		t.Fatal("expected error when target file exists; got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already-exists, got: %v", err)
	}

	// Operator file untouched.
	body, _ := os.ReadFile(path)
	if string(body) != "# operator-supplied" {
		t.Errorf("operator file mutated: %s", body)
	}
}

func TestWriteEnvTemplate_NoClobber(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("CLM_TRUENAS_API_KEY=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteEnvTemplateIfMissing(envPath); err != nil {
		t.Fatalf("WriteEnvTemplateIfMissing: %v", err)
	}

	got, _ := os.ReadFile(envPath)
	if !strings.Contains(string(got), "CLM_TRUENAS_API_KEY=secret") {
		t.Errorf("operator-supplied .env was overwritten: %s", got)
	}
}

func TestWriteEnvTemplate_CreatesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "data", ".env")

	if err := WriteEnvTemplateIfMissing(envPath); err != nil {
		t.Fatalf("WriteEnvTemplateIfMissing: %v", err)
	}

	if _, err := os.Stat(envPath); err != nil {
		t.Fatalf("expected .env created at %s: %v", envPath, err)
	}
}

func TestWriteComposeFile_RejectsEmptyImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	err := WriteComposeFile(path, ComposeParams{
		MainImage:    "",
		MainService:  "clubfoundry",
		UpdaterImage: "clubfoundry-updater:dev",
	})
	if err == nil {
		t.Fatal("expected error for empty MainImage")
	}
}

func TestEnsureProjectNamePatchesOnceWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	original := "# operator compose\nservices:\n  app:\n    image: example:1\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureProjectName(path, "clubfoundry")
	if err != nil || !changed {
		t.Fatalf("EnsureProjectName() = (%v, %v), want (true, nil)", changed, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(body)
	nameAt := strings.Index(out, "name: clubfoundry")
	servicesAt := strings.Index(out, "services:")
	if nameAt < 0 || servicesAt < 0 || nameAt > servicesAt {
		t.Fatalf("project anchor is missing or after services:\n%s", out)
	}
	bakPath := path + ".bak-clubfoundry-anchor"
	bak, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(bak) != original {
		t.Fatalf("backup changed: %q", bak)
	}

	changed, err = EnsureProjectName(path, "clubfoundry")
	if err != nil || changed {
		t.Fatalf("second EnsureProjectName() = (%v, %v), want (false, nil)", changed, err)
	}
}

func TestEnsureProjectNameMissingFileIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-compose.yml")
	changed, err := EnsureProjectName(path, "clubfoundry")
	if err != nil || changed {
		t.Fatalf("EnsureProjectName(missing) = (%v, %v), want (false, nil)", changed, err)
	}
}

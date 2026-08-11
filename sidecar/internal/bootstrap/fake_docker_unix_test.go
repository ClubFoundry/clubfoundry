//go:build !windows

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFakeDocker(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-docker")
	body := []byte("#!/bin/sh\n" +
		"if [ -n \"$FAKE_DOCKER_STDOUT\" ]; then printf '%s\\n' \"$FAKE_DOCKER_STDOUT\"; fi\n" +
		"if [ -n \"$FAKE_DOCKER_STDOUT_2\" ]; then printf '%s\\n' \"$FAKE_DOCKER_STDOUT_2\"; fi\n" +
		"exit \"${FAKE_DOCKER_EXIT:-0}\"\n")
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

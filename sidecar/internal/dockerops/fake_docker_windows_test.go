//go:build windows

package dockerops

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFakeDocker(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-docker.cmd")
	body := []byte("@echo off\r\n" +
		"if not \"%FAKE_DOCKER_ARGS_FILE%\"==\"\" echo %*>>\"%FAKE_DOCKER_ARGS_FILE%\"\r\n" +
		"echo(%FAKE_DOCKER_STDOUT%\r\n" +
		"echo(%FAKE_DOCKER_STDOUT_2%\r\n" +
		"if defined FAKE_DOCKER_EXIT exit /b %FAKE_DOCKER_EXIT%\r\n" +
		"exit /b 0\r\n")
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

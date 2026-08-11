package dockerops

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// loadImageFromFile imports one verified artifact and updates Compose intent.
func (c Config) loadImageFromFile(ctx context.Context, service, tag, path string, opts PullOpts) error {
	if opts.OnLoadStart != nil {
		opts.OnLoadStart()
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open downloaded image %s: %w", path, err)
	}
	defer f.Close()

	loadCmd := exec.CommandContext(ctx, c.DockerBin, "load")
	loadCmd.Stdin = f
	var loadOut bytes.Buffer
	if opts.LogWriter != nil {
		loadCmd.Stdout = io.MultiWriter(&loadOut, opts.LogWriter)
		loadCmd.Stderr = io.MultiWriter(&loadOut, opts.LogWriter)
	} else {
		loadCmd.Stdout = &loadOut
		loadCmd.Stderr = &loadOut
	}
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("docker load from %s: %w: %s", path, classifyDownloadErr(err), loadOut.String())
	}

	loaded := parseLoadedImage(loadOut.String())
	if loaded == "" {
		return fmt.Errorf("docker load succeeded but no image tag parsed from: %s", loadOut.String())
	}
	writeLog(opts.LogWriter, "loaded image: %s\n", loaded)

	finalRef := loaded
	if tag != "" && !strings.Contains(loaded, ":"+tag) {
		finalRef = tag
	}
	if service != "" {
		if err := c.SetServiceImage(service, finalRef); err != nil {
			return fmt.Errorf("set image to %s after load: %w", finalRef, err)
		}
		writeLog(opts.LogWriter, "compose image: line rewritten → %s\n", finalRef)
	}
	return nil
}

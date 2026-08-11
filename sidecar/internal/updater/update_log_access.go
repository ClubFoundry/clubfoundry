package updater

import (
	"io"
	"path/filepath"
)

func (u *updateLog) writer() io.Writer {
	if u == nil || u.text == nil {
		return io.Discard
	}
	return u.text
}

func (u *updateLog) path() string {
	if u == nil {
		return ""
	}
	return u.dir
}

func (u *updateLog) logFilePath() string {
	if u == nil || u.dir == "" {
		return ""
	}
	return filepath.Join(u.dir, "update.log")
}

func (u *updateLog) close() {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return
	}
	u.closed = true
	if u.text != nil {
		_ = u.text.Sync()
		_ = u.text.Close()
		u.text = nil
	}
	if u.phases != nil {
		_ = u.phases.Sync()
		_ = u.phases.Close()
		u.phases = nil
	}
}

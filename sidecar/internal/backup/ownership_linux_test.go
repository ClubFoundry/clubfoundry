//go:build linux

package backup

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCopyFilePreservesDstOwnership(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "live.db")
	src := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(dst, sqliteContent("-old"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	if err := os.WriteFile(src, sqliteContent("-new"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	stBefore := dstInfo.Sys().(*syscall.Stat_t)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst after copy: %v", err)
	}
	if string(got) != string(sqliteContent("-new")) {
		t.Errorf("content not copied: got %q, want %q", got, sqliteContent("-new"))
	}
	dstInfoAfter, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst after: %v", err)
	}
	stAfter := dstInfoAfter.Sys().(*syscall.Stat_t)
	if stAfter.Uid != stBefore.Uid || stAfter.Gid != stBefore.Gid {
		t.Errorf("ownership changed: before=%d:%d, after=%d:%d",
			stBefore.Uid, stBefore.Gid, stAfter.Uid, stAfter.Gid)
	}
}

func TestCopyFilePreservesNonRootDstOwnershipWhenCalledAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root to change file ownership")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "live.db")
	src := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(dst, sqliteContent("-old"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	if err := os.WriteFile(src, sqliteContent("-new"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	const targetUID, targetGID = 100, 101
	if err := os.Chown(dst, targetUID, targetGID); err != nil {
		t.Fatalf("chown dst to %d:%d: %v", targetUID, targetGID, err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	post, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst after: %v", err)
	}
	stPost := post.Sys().(*syscall.Stat_t)
	if stPost.Uid != targetUID || stPost.Gid != targetGID {
		t.Errorf("ownership changed: got %d:%d, want %d:%d",
			stPost.Uid, stPost.Gid, targetUID, targetGID)
	}
}

package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sqliteHeader is what every SQLite 3 file opens with. Test fixtures use it
// as a content prefix so validateSQLiteHeader accepts realistic main files.
var sqliteHeader = []byte("SQLite format 3\x00")

func sqliteContent(suffix string) []byte {
	return append(append([]byte{}, sqliteHeader...), []byte(suffix)...)
}

func TestCreateRestoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "clm.db")
	v1 := sqliteContent("-v1-contents")
	if err := os.WriteFile(dbPath, v1, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := Config{DBPath: dbPath, BackupsDir: filepath.Join(dir, "backups"), KeepN: 3}
	backupPath, err := c.CreateBackup("1.0.20")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// Simulate update: overwrite the DB with new contents.
	if err := os.WriteFile(dbPath, sqliteContent("-v2-broken"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	// Rollback.
	if err := c.RestoreBackup(backupPath); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(v1) {
		t.Fatalf("restore contents: got %q, want %q", got, v1)
	}
}

func TestPruneKeepsMostRecent(t *testing.T) {
	dir := t.TempDir()
	backupsDir := filepath.Join(dir, "backups")
	_ = os.MkdirAll(backupsDir, 0o755)

	// Create 6 main backup files with staggered mtimes. Each carries the
	// SQLite magic so the file shape is realistic; PruneOld doesn't read
	// content but the parallel WAL/SHM siblings test (TestPruneDoesNotPickShmAsLatest)
	// downstream depends on these being valid restore targets.
	base := time.Now().Add(-6 * time.Hour)
	for i := 0; i < 6; i++ {
		p := filepath.Join(backupsDir, formatBackupName(i))
		if err := os.WriteFile(p, sqliteContent("-x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := base.Add(time.Duration(i) * time.Hour)
		_ = os.Chtimes(p, mod, mod)
	}

	c := Config{BackupsDir: backupsDir, KeepN: 3}
	if err := c.PruneOld(); err != nil {
		t.Fatalf("PruneOld: %v", err)
	}
	entries, _ := os.ReadDir(backupsDir)
	if len(entries) != 3 {
		t.Fatalf("want 3 remaining, got %d", len(entries))
	}
}

func formatBackupName(seq int) string {
	return "clm.db.pre-update-v1.0." + string(rune('0'+seq)) + "-test"
}

// TestWalTripletRoundtrip verifies that WAL and SHM siblings travel with the
// main database through backup and restore. A main-file-only copy may contain
// only the page-table shell while current data remains in the WAL.
func TestWalTripletRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "clm.db")
	if err := os.WriteFile(dbPath, sqliteContent("-page-table-v1"), 0o644); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	if err := os.WriteFile(dbPath+"-wal", []byte("wal-pages-v1"), 0o644); err != nil {
		t.Fatalf("seed wal: %v", err)
	}
	if err := os.WriteFile(dbPath+"-shm", []byte("shm-v1"), 0o644); err != nil {
		t.Fatalf("seed shm: %v", err)
	}

	c := Config{DBPath: dbPath, BackupsDir: filepath.Join(dir, "backups"), KeepN: 3}
	backupPath, err := c.CreateBackup("1.0.30")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(backupPath + suffix); err != nil {
			t.Fatalf("expected backup sibling %s, got: %v", backupPath+suffix, err)
		}
	}

	// Simulate a bad update: overwrite all three.
	_ = os.WriteFile(dbPath, sqliteContent("-page-table-v2-broken"), 0o644)
	_ = os.WriteFile(dbPath+"-wal", []byte("wal-v2-broken"), 0o644)
	_ = os.WriteFile(dbPath+"-shm", []byte("shm-v2-broken"), 0o644)

	if err := c.RestoreBackup(backupPath); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	for _, pair := range [][2]string{
		{"", string(sqliteContent("-page-table-v1"))},
		{"-wal", "wal-pages-v1"},
		{"-shm", "shm-v1"},
	} {
		got, err := os.ReadFile(dbPath + pair[0])
		if err != nil {
			t.Fatalf("read %s: %v", dbPath+pair[0], err)
		}
		if string(got) != pair[1] {
			t.Fatalf("restored %s contents: got %q, want %q", dbPath+pair[0], got, pair[1])
		}
	}
}

// TestRestoreWithoutWalSiblingClearsLiveWal — when the backup predates WAL
// mode (or was a checkpointed clean copy), restore must NOT leave the
// live -wal file in place, otherwise SQLite would try to apply the stale
// WAL to the restored page table on next open.
func TestRestoreWithoutWalSiblingClearsLiveWal(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "clm.db")
	_ = os.WriteFile(dbPath, sqliteContent("-page-table-v1"), 0o644)
	// NOTE: no WAL/SHM seed — backup will only capture the .db file.

	c := Config{DBPath: dbPath, BackupsDir: filepath.Join(dir, "backups"), KeepN: 3}
	backupPath, err := c.CreateBackup("1.0.30")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Now simulate a live WAL+SHM appearing between backup and restore.
	_ = os.WriteFile(dbPath+"-wal", []byte("stale-wal"), 0o644)
	_ = os.WriteFile(dbPath+"-shm", []byte("stale-shm"), 0o644)

	if err := c.RestoreBackup(backupPath); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if _, err := os.Stat(dbPath + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("expected live -wal to be removed; err=%v", err)
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Fatalf("expected live -shm to be removed; err=%v", err)
	}
}

func TestSanitize(t *testing.T) {
	got := sanitize("1.0.30-rc/1")
	if got != "1.0.30-rc_1" {
		t.Errorf("sanitize: got %q", got)
	}
}

// TestIsMainBackupNameRejectsSiblings verifies that LatestBackup cannot treat
// a newer WAL or SHM sibling as the main database restore target.
func TestIsMainBackupNameRejectsSiblings(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"clm.db.pre-update-v1.2.4-20260510T100000Z", true},
		{"clm.db.pre-update-v1.2.4-20260510T100000Z-wal", false},
		{"clm.db.pre-update-v1.2.4-20260510T100000Z-shm", false},
		{"clm.db.pre-update-v1.2.4-20260510T100000Z.part", false},
		{"clm.db", false},
		{"clm.db.dryrun-1778408495", false},
		{"unrelated", false},
	}
	for _, tc := range cases {
		if got := isMainBackupName(tc.name); got != tc.want {
			t.Errorf("isMainBackupName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestLatestBackupIgnoresShmAndWal covers a main backup followed by newer WAL
// and SHM siblings, matching the normal triplet copy order.
func TestLatestBackupIgnoresShmAndWal(t *testing.T) {
	dir := t.TempDir()
	backupsDir := filepath.Join(dir, "backups")
	_ = os.MkdirAll(backupsDir, 0o755)

	mainName := "clm.db.pre-update-v1.2.4-20260510T100000Z"
	mainPath := filepath.Join(backupsDir, mainName)
	walPath := mainPath + "-wal"
	shmPath := mainPath + "-shm"
	for _, p := range []string{mainPath, walPath, shmPath} {
		if err := os.WriteFile(p, sqliteContent("-fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Make WAL + SHM newer than main, exactly as cp triplet does in production.
	now := time.Now()
	_ = os.Chtimes(mainPath, now.Add(-2*time.Second), now.Add(-2*time.Second))
	_ = os.Chtimes(walPath, now.Add(-1*time.Second), now.Add(-1*time.Second))
	_ = os.Chtimes(shmPath, now, now)

	c := Config{BackupsDir: backupsDir, KeepN: 3}
	got, err := c.LatestBackup()
	if err != nil {
		t.Fatalf("LatestBackup: %v", err)
	}
	if got != mainPath {
		t.Errorf("LatestBackup picked %q, want %q (the SHM sibling must be skipped)", got, mainPath)
	}
}

func TestVersionFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "semantic version",
			path: "/app/data/backups/clm.db.pre-update-v1.3.136-20260808T101426Z",
			want: "1.3.136",
		},
		{
			name: "version with dash",
			path: "/app/data/backups/clm.db.pre-update-v1.3.138-alpha_1-20260808T101426Z",
			want: "1.3.138-alpha_1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := VersionFromPath(tt.path)
			if err != nil {
				t.Fatalf("VersionFromPath: %v", err)
			}
			if got != tt.want {
				t.Fatalf("VersionFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestVersionFromPathRejectsMalformedName(t *testing.T) {
	for _, path := range []string{
		"clm.db",
		"clm.db.pre-update-v1.3.136",
		"clm.db.pre-update-v1.3.136-not-a-timestamp",
		"clm.db.pre-update-v1.3.136-20260808T101426Z-wal",
	} {
		if _, err := VersionFromPath(path); err == nil {
			t.Fatalf("VersionFromPath(%q) unexpectedly succeeded", path)
		}
	}
}

// TestRestoreBackupRejectsNonSqliteFile is the second-line defense against
// the same class of bugs: even if a future regression slipped a non-DB
// path past the name filter, the SQLite-magic check would refuse to
// overwrite the live DB.
func TestRestoreBackupRejectsNonSqliteFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "clm.db")
	originalContent := sqliteContent("-original-live-db")
	if err := os.WriteFile(dbPath, originalContent, 0o644); err != nil {
		t.Fatalf("seed live db: %v", err)
	}

	// A file that looks like a backup but lacks the SQLite magic simulates a
	// sibling path, truncated artifact, or partial copy reaching the guard.
	bogusPath := filepath.Join(dir, "fake-backup")
	if err := os.WriteFile(bogusPath, []byte{0x18, 0xe2, 0x2d, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf2, 0x02, 0x00, 0x00, 0x01, 0x00, 0x00, 0x10}, 0o644); err != nil {
		t.Fatal(err)
	}

	c := Config{DBPath: dbPath, BackupsDir: filepath.Join(dir, "backups"), KeepN: 3}
	err := c.RestoreBackup(bogusPath)
	if err == nil {
		t.Fatal("RestoreBackup must REFUSE non-SQLite file, but returned nil")
	}
	if !strings.Contains(err.Error(), "not a SQLite database") {
		t.Errorf("error should mention the SQLite header check, got: %v", err)
	}
	// Live DB must be untouched.
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read live db after refused restore: %v", err)
	}
	if string(got) != string(originalContent) {
		t.Errorf("live db was MODIFIED by failed restore — defensive copy guarantee broken: got %q, want %q", got, originalContent)
	}
}

// TestPruneRemovesSiblingsTogether — when a main backup is pruned, its
// -wal / -shm siblings must go too. Otherwise next iteration's
// LatestBackup() (post-fix) is still safe, but the orphan files waste
// disk + visually confuse operators.
func TestPruneRemovesSiblingsTogether(t *testing.T) {
	dir := t.TempDir()
	backupsDir := filepath.Join(dir, "backups")
	_ = os.MkdirAll(backupsDir, 0o755)

	base := time.Now().Add(-6 * time.Hour)
	for i := 0; i < 4; i++ {
		mainName := "clm.db.pre-update-v1.2." + string(rune('0'+i)) + "-test"
		mainPath := filepath.Join(backupsDir, mainName)
		_ = os.WriteFile(mainPath, sqliteContent("-x"), 0o644)
		_ = os.WriteFile(mainPath+"-wal", []byte("wal"), 0o644)
		_ = os.WriteFile(mainPath+"-shm", []byte("shm"), 0o644)
		mod := base.Add(time.Duration(i) * time.Hour)
		_ = os.Chtimes(mainPath, mod, mod)
		_ = os.Chtimes(mainPath+"-wal", mod, mod)
		_ = os.Chtimes(mainPath+"-shm", mod, mod)
	}

	c := Config{BackupsDir: backupsDir, KeepN: 2}
	if err := c.PruneOld(); err != nil {
		t.Fatalf("PruneOld: %v", err)
	}
	entries, _ := os.ReadDir(backupsDir)
	// Expect 6 entries: 2 main + 2 wal + 2 shm (the two newest triplets).
	if len(entries) != 6 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("want 6 remaining (2 triplets), got %d: %v", len(entries), names)
	}
}

// TestCleanOrphanSiblings verifies that PruneOld removes WAL and SHM files
// whose main backup is already missing.
func TestCleanOrphanSiblings(t *testing.T) {
	dir := t.TempDir()
	backupsDir := filepath.Join(dir, "backups")
	_ = os.MkdirAll(backupsDir, 0o755)

	// Layout:
	//   clm.db.pre-update-v1.2.2-A          ← orphan (no main)
	//   clm.db.pre-update-v1.2.2-A-wal      ← orphan
	//   clm.db.pre-update-v1.2.3-B          ← OK (main present)
	//   clm.db.pre-update-v1.2.3-B-wal
	// Seed only the v1.2.2 siblings to represent an orphaned triplet.
	_ = os.WriteFile(filepath.Join(backupsDir, "clm.db.pre-update-v1.2.2-A-wal"), []byte("wal"), 0o644)
	_ = os.WriteFile(filepath.Join(backupsDir, "clm.db.pre-update-v1.2.2-A-shm"), []byte("shm"), 0o644)
	mainV3 := filepath.Join(backupsDir, "clm.db.pre-update-v1.2.3-B")
	_ = os.WriteFile(mainV3, sqliteContent("-x"), 0o644)
	_ = os.WriteFile(mainV3+"-wal", []byte("wal"), 0o644)
	_ = os.WriteFile(mainV3+"-shm", []byte("shm"), 0o644)

	c := Config{BackupsDir: backupsDir, KeepN: 5}
	if err := c.PruneOld(); err != nil {
		t.Fatalf("PruneOld: %v", err)
	}

	// Orphan -wal/-shm must be gone; the v1.2.3 triplet preserved.
	for _, want := range []struct {
		name        string
		shouldExist bool
	}{
		{"clm.db.pre-update-v1.2.2-A-wal", false},
		{"clm.db.pre-update-v1.2.2-A-shm", false},
		{"clm.db.pre-update-v1.2.3-B", true},
		{"clm.db.pre-update-v1.2.3-B-wal", true},
		{"clm.db.pre-update-v1.2.3-B-shm", true},
	} {
		_, err := os.Stat(filepath.Join(backupsDir, want.name))
		exists := err == nil
		if exists != want.shouldExist {
			t.Errorf("%s: exists=%v, want=%v (err=%v)", want.name, exists, want.shouldExist, err)
		}
	}
}

// A new destination has no ownership to preserve. copyFile must still create
// it with the operating-system default owner and the expected contents.
func TestCopyFileToNewDstSkipsChown(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	dst := filepath.Join(dir, "subdir", "new.db")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(src, sqliteContent("-content"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	// dst does NOT exist yet — copyFile should write src content under
	// the OS default ownership without erroring on the chown step.
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile to new dst: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(sqliteContent("-content")) {
		t.Errorf("content mismatch: got %q, want %q", got, sqliteContent("-content"))
	}
}

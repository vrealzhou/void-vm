package vmctl

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupFileCreatesCorrectBackup(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "test-pair"
	timestamp := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "project", "main.go")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	content := []byte("package main\n")
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	backupPath, err := backupFile(srcFile, backupsDir, pairID, timestamp)
	if err != nil {
		t.Fatalf("backupFile failed: %v", err)
	}

	wantPath := filepath.Join(backupsDir, pairID, timestamp.Format(time.RFC3339), "main.go")
	if backupPath != wantPath {
		t.Fatalf("backupPath = %q, want %q", backupPath, wantPath)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}

	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("backup content = %q, want %q", got, content)
	}
}

func TestBackupDirectoryRecursively(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "pair-2"
	timestamp := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	srcDir := t.TempDir()
	files := map[string]string{
		"root.txt":              "root content",
		"sub/a.txt":             "a content",
		"sub/nested/b.txt":      "b content",
		"another/c.txt":         "c content",
	}
	for rel, content := range files {
		path := filepath.Join(srcDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	if err := backupDirectory(srcDir, backupsDir, pairID, timestamp); err != nil {
		t.Fatalf("backupDirectory failed: %v", err)
	}

	backupBase := filepath.Join(backupsDir, pairID, timestamp.Format(time.RFC3339))
	for rel, wantContent := range files {
		backupFile := filepath.Join(backupBase, rel)
		got, err := os.ReadFile(backupFile)
		if err != nil {
			t.Fatalf("backup file %q missing: %v", rel, err)
		}
		if string(got) != wantContent {
			t.Fatalf("backup file %q content = %q, want %q", rel, got, wantContent)
		}
	}
}

func TestCleanupOldBackups(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "pair-3"
	now := time.Now().UTC()

	timestamps := []time.Time{
		now.Add(-48 * time.Hour), // old
		now.Add(-24 * time.Hour), // old
		now.Add(-1 * time.Hour),  // recent
		now,                      // recent
	}
	for _, ts := range timestamps {
		dir := filepath.Join(backupsDir, pairID, ts.Format(time.RFC3339))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create backup dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dummy.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to write dummy: %v", err)
		}
	}

	if err := cleanupOldBackups(backupsDir, pairID, 1); err != nil {
		t.Fatalf("cleanupOldBackups failed: %v", err)
	}

	remaining, err := listBackups(backupsDir, pairID)
	if err != nil {
		t.Fatalf("listBackups failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining backups, got %d", len(remaining))
	}

	for _, ts := range remaining {
		if ts.Before(now.Add(-2 * time.Hour)) {
			t.Fatalf("old backup %v should have been removed", ts)
		}
	}
}

func TestListBackupsSortedNewestFirst(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "pair-4"

	timestamps := []time.Time{
		time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	for _, ts := range timestamps {
		dir := filepath.Join(backupsDir, pairID, ts.Format(time.RFC3339))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create backup dir: %v", err)
		}
	}

	got, err := listBackups(backupsDir, pairID)
	if err != nil {
		t.Fatalf("listBackups failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(got))
	}

	want := []time.Time{
		time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("backup[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRestoreBackup(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "pair-5"
	timestamp := time.Date(2025, 1, 15, 14, 0, 0, 0, time.UTC)

	backupBase := filepath.Join(backupsDir, pairID, timestamp.Format(time.RFC3339))
	files := map[string]string{
		"file1.txt":        "content1",
		"dir/file2.txt":    "content2",
		"deep/nested/f.txt": "content3",
	}
	for rel, content := range files {
		path := filepath.Join(backupBase, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	targetDir := t.TempDir()
	if err := restoreBackup(backupsDir, pairID, timestamp, targetDir); err != nil {
		t.Fatalf("restoreBackup failed: %v", err)
	}

	for rel, wantContent := range files {
		path := filepath.Join(targetDir, rel)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("restored file %q missing: %v", rel, err)
		}
		if string(got) != wantContent {
			t.Fatalf("restored file %q content = %q, want %q", rel, got, wantContent)
		}
	}
}

func TestListBackupsEmpty(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "pair-empty"

	got, err := listBackups(backupsDir, pairID)
	if err != nil {
		t.Fatalf("listBackups failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 backups, got %d", len(got))
	}
}

func TestListBackupsIgnoresNonTimestampDirs(t *testing.T) {
	backupsDir := t.TempDir()
	pairID := "pair-bad"

	valid := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	if err := os.MkdirAll(filepath.Join(backupsDir, pairID, valid.Format(time.RFC3339)), 0o755); err != nil {
		t.Fatalf("failed to create valid dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(backupsDir, pairID, "not-a-timestamp"), 0o755); err != nil {
		t.Fatalf("failed to create invalid dir: %v", err)
	}

	got, err := listBackups(backupsDir, pairID)
	if err != nil {
		t.Fatalf("listBackups failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(got))
	}
	if !got[0].Equal(valid) {
		t.Fatalf("backup = %v, want %v", got[0], valid)
	}
}

func TestBuildRsyncArgsHostToVM(t *testing.T) {
	cfg := Config{
		SSHUser:  "dev",
		StaticIP: "192.168.64.10",
	}
	pair := SyncPair{
		ID:       "myapp",
		HostPath: "/Users/dev/myapp",
		VMPath:   "/home/dev/myapp",
		Exclude:  []string{"*.tmp", ".DS_Store"},
	}

	args := buildRsyncArgs(cfg, pair, true)

	want := []string{
		"-avz", "--delete",
		"-e", "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
		"--exclude", "*.tmp",
		"--exclude", ".DS_Store",
		"/Users/dev/myapp/", "dev@192.168.64.10:/home/dev/myapp/",
	}

	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d; args = %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildRsyncArgsVMToHost(t *testing.T) {
	cfg := Config{
		SSHUser:  "dev",
		StaticIP: "192.168.64.10",
	}
	pair := SyncPair{
		ID:          "docs",
		HostPath:    "/Users/dev/docs",
		VMPath:      "/home/dev/docs",
		ExcludeFrom: "/tmp/exclude.txt",
	}

	args := buildRsyncArgs(cfg, pair, false)

	want := []string{
		"-avz", "--delete",
		"-e", "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
		"--exclude-from", "/tmp/exclude.txt",
		"dev@192.168.64.10:/home/dev/docs/", "/Users/dev/docs/",
	}

	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d; args = %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestRetentionDaysDefault(t *testing.T) {
	pair := SyncPair{BackupRetentionDays: 0}
	if got := retentionDays(pair); got != 7 {
		t.Fatalf("retentionDays(default) = %d, want 7", got)
	}
}

func TestRetentionDaysCustom(t *testing.T) {
	pair := SyncPair{BackupRetentionDays: 14}
	if got := retentionDays(pair); got != 14 {
		t.Fatalf("retentionDays(custom) = %d, want 14", got)
	}
}

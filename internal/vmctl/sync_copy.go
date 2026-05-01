package vmctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

func backupFile(srcPath, backupsDir, pairID string, timestamp time.Time) (string, error) {
	backupDir := filepath.Join(backupsDir, pairID, timestamp.Format(time.RFC3339))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	backupPath := filepath.Join(backupDir, filepath.Base(srcPath))
	if err := copyFile(srcPath, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func backupDirectory(srcDir, backupsDir, pairID string, timestamp time.Time) error {
	backupBase := filepath.Join(backupsDir, pairID, timestamp.Format(time.RFC3339))
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(backupBase, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFile(path, dst)
	})
}

func cleanupOldBackups(backupsDir, pairID string, retentionDays int) error {
	pairDir := filepath.Join(backupsDir, pairID)
	entries, err := os.ReadDir(pairDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ts, err := time.Parse(time.RFC3339, entry.Name())
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(pairDir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func listBackups(backupsDir, pairID string) ([]time.Time, error) {
	pairDir := filepath.Join(backupsDir, pairID)
	entries, err := os.ReadDir(pairDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ts, err := time.Parse(time.RFC3339, entry.Name())
		if err != nil {
			continue
		}
		result = append(result, ts)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].After(result[j])
	})
	return result, nil
}

func restoreBackup(backupsDir, pairID string, timestamp time.Time, targetDir string) error {
	backupDir := filepath.Join(backupsDir, pairID, timestamp.Format(time.RFC3339))
	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(backupDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(targetDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFile(path, dst)
	})
}

func retentionDays(pair SyncPair) int {
	if pair.BackupRetentionDays != 0 {
		return pair.BackupRetentionDays
	}
	return 7
}

func buildRsyncArgs(cfg Config, pair SyncPair, hostToVM bool) []string {
	args := []string{
		"-avz",
		"--delete",
		"-e", "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
	}

	for _, pattern := range pair.Exclude {
		args = append(args, "--exclude", pattern)
	}
	if pair.ExcludeFrom != "" {
		args = append(args, "--exclude-from", pair.ExcludeFrom)
	}

	if hostToVM {
		args = append(args, pair.HostPath+"/", cfg.SSHUser+"@"+cfg.StaticIP+":"+pair.VMPath+"/")
	} else {
		args = append(args, cfg.SSHUser+"@"+cfg.StaticIP+":"+pair.VMPath+"/", pair.HostPath+"/")
	}

	return args
}

func runRsync(args []string) error {
	cmd := exec.Command("rsync", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CopySyncHostToVM syncs from host to VM (no backup needed since we're not overwriting host files)
func CopySyncHostToVM(cfg Config, pair SyncPair, backupsDir string) error {
	if err := cleanupOldBackups(backupsDir, pair.ID, retentionDays(pair)); err != nil {
		return fmt.Errorf("cleanup old backups: %w", err)
	}
	args := buildRsyncArgs(cfg, pair, true)
	return runRsync(args)
}

// CopySyncVMToHost syncs from VM to host (backup host files before overwriting)
func CopySyncVMToHost(cfg Config, pair SyncPair, backupsDir string) error {
	if err := cleanupOldBackups(backupsDir, pair.ID, retentionDays(pair)); err != nil {
		return fmt.Errorf("cleanup old backups: %w", err)
	}

	timestamp := time.Now().UTC()
	if err := backupDirectory(pair.HostPath, backupsDir, pair.ID, timestamp); err != nil {
		return fmt.Errorf("backup host dir: %w", err)
	}

	args := buildRsyncArgs(cfg, pair, false)
	return runRsync(args)
}

// CopySyncBidirectional performs bidirectional sync: VM->Host first (with backup), then Host->VM
func CopySyncBidirectional(cfg Config, pair SyncPair, backupsDir string) error {
	if err := CopySyncVMToHost(cfg, pair, backupsDir); err != nil {
		return fmt.Errorf("vm to host: %w", err)
	}
	if err := CopySyncHostToVM(cfg, pair, backupsDir); err != nil {
		return fmt.Errorf("host to vm: %w", err)
	}
	return nil
}

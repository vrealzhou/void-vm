package vmctl

import (
	"os"
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

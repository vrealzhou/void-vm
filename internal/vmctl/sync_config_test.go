package vmctl

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSyncConfigNonExistent(t *testing.T) {
	cfg, err := LoadSyncConfig(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 0 {
		t.Fatalf("expected version 0, got %d", cfg.Version)
	}
	if len(cfg.Pairs) != 0 {
		t.Fatalf("expected empty pairs, got %d", len(cfg.Pairs))
	}
}

func TestLoadSaveSyncConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	original := SyncConfig{
		Version: 1,
		Pairs: []SyncPair{
			{
				ID:        "pair-1",
				Mode:      SyncModeGit,
				HostPath:  "/host/project",
				VMPath:    "/vm/project",
				Direction: SyncDirectionHostToVM,
				Exclude:   []string{".git", "node_modules"},
				CreatedAt: now,
			},
			{
				ID:                  "pair-2",
				Mode:                SyncModeCopy,
				HostPath:            "/host/data",
				VMPath:              "/vm/data",
				BareRepoPath:        "/vm/data.git",
				Direction:           SyncDirectionBidirectional,
				ExcludeFrom:         "/host/.syncignore",
				BackupRetentionDays: 7,
				CreatedAt:           now.Add(time.Hour),
			},
		},
	}

	yamlPath := filepath.Join(dir, "vmctl.yaml")
	if err := SaveSyncConfig(yamlPath, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadSyncConfig(yamlPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded.Pairs) != len(original.Pairs) {
		t.Fatalf("pairs length mismatch: got %d want %d", len(loaded.Pairs), len(original.Pairs))
	}

	for i, want := range original.Pairs {
		got := loaded.Pairs[i]
		if got.ID != want.ID {
			t.Fatalf("pair[%d].ID: got %q want %q", i, got.ID, want.ID)
		}
		if got.Mode != want.Mode {
			t.Fatalf("pair[%d].Mode: got %q want %q", i, got.Mode, want.Mode)
		}
		if got.HostPath != want.HostPath {
			t.Fatalf("pair[%d].HostPath: got %q want %q", i, got.HostPath, want.HostPath)
		}
		if got.VMPath != want.VMPath {
			t.Fatalf("pair[%d].VMPath: got %q want %q", i, got.VMPath, want.VMPath)
		}
		if got.BareRepoPath != want.BareRepoPath {
			t.Fatalf("pair[%d].BareRepoPath: got %q want %q", i, got.BareRepoPath, want.BareRepoPath)
		}
		if got.Direction != want.Direction {
			t.Fatalf("pair[%d].Direction: got %q want %q", i, got.Direction, want.Direction)
		}
		if got.ExcludeFrom != want.ExcludeFrom {
			t.Fatalf("pair[%d].ExcludeFrom: got %q want %q", i, got.ExcludeFrom, want.ExcludeFrom)
		}
		if got.BackupRetentionDays != want.BackupRetentionDays {
			t.Fatalf("pair[%d].BackupRetentionDays: got %d want %d", i, got.BackupRetentionDays, want.BackupRetentionDays)
		}
		if len(got.Exclude) != len(want.Exclude) {
			t.Fatalf("pair[%d].Exclude length: got %d want %d", i, len(got.Exclude), len(want.Exclude))
		}
		for j := range want.Exclude {
			if got.Exclude[j] != want.Exclude[j] {
				t.Fatalf("pair[%d].Exclude[%d]: got %q want %q", i, j, got.Exclude[j], want.Exclude[j])
			}
		}
	}
}

func TestSyncConfigGetPair(t *testing.T) {
	cfg := SyncConfig{
		Pairs: []SyncPair{
			{ID: "a", HostPath: "/host/a"},
			{ID: "b", HostPath: "/host/b"},
		},
	}

	pair, ok := cfg.GetPair("a")
	if !ok {
		t.Fatal("expected to find pair a")
	}
	if pair.ID != "a" {
		t.Fatalf("unexpected pair ID: %q", pair.ID)
	}

	_, ok = cfg.GetPair("c")
	if ok {
		t.Fatal("expected not to find pair c")
	}
}

func TestSyncConfigAddPair(t *testing.T) {
	cfg := SyncConfig{Version: 1}

	p1 := SyncPair{ID: "p1", HostPath: "/host/p1"}
	if err := cfg.AddPair(p1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(cfg.Pairs))
	}

	if err := cfg.AddPair(p1); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestSyncConfigRemovePair(t *testing.T) {
	cfg := SyncConfig{
		Pairs: []SyncPair{
			{ID: "a", HostPath: "/host/a"},
			{ID: "b", HostPath: "/host/b"},
		},
	}

	if !cfg.RemovePair("a") {
		t.Fatal("expected RemovePair(a) to return true")
	}
	if len(cfg.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(cfg.Pairs))
	}
	if cfg.Pairs[0].ID != "b" {
		t.Fatalf("expected remaining pair to be b, got %q", cfg.Pairs[0].ID)
	}

	if cfg.RemovePair("a") {
		t.Fatal("expected RemovePair(a) to return false")
	}
}

func TestLoadSyncConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSyncConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

package vmctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBareRepoPath(t *testing.T) {
	cfg := Config{
		GuestUser: "vm",
	}
	pair := SyncPair{
		HostPath: "/Users/vm/projects/myapp",
	}
	got := defaultBareRepoPath(cfg, pair)
	want := "/home/vm/repos/myapp/repo.git"
	if got != want {
		t.Errorf("defaultBareRepoPath() = %q, want %q", got, want)
	}
}

func TestGitRemoteURL(t *testing.T) {
	cfg := Config{
		SSHUser:   "vm",
		StaticIP:  "192.168.64.10",
		GuestUser: "vm",
	}
	pair := SyncPair{
		BareRepoPath: "/home/vm/repos/myapp/repo.git",
	}
	got := gitRemoteURL(cfg, pair)
	want := "ssh://vm@192.168.64.10/~/repos/myapp/repo.git"
	if got != want {
		t.Errorf("gitRemoteURL() = %q, want %q", got, want)
	}
}

func TestIsGitRepo(t *testing.T) {
	t.Run("returns true for git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("failed to create .git dir: %v", err)
		}
		if !IsGitRepo(tmpDir) {
			t.Errorf("IsGitRepo(%q) = false, want true", tmpDir)
		}
	})

	t.Run("returns false for non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		if IsGitRepo(tmpDir) {
			t.Errorf("IsGitRepo(%q) = true, want false", tmpDir)
		}
	})

	t.Run("returns false for non-existent path", func(t *testing.T) {
		if IsGitRepo("/nonexistent/path/12345") {
			t.Errorf("IsGitRepo(nonexistent) = true, want false")
		}
	})
}

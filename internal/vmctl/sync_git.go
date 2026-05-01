package vmctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultBareRepoPath generates the default bare repo path inside the VM
// based on the host path's basename.
func defaultBareRepoPath(cfg Config, pair SyncPair) string {
	base := filepath.Base(pair.HostPath)
	return "/home/" + cfg.GuestUser + "/repos/" + base + "/repo.git"
}

// gitRemoteURL generates the SSH remote URL for the bare repo on the VM.
func gitRemoteURL(cfg Config, pair SyncPair) string {
	relPath := strings.TrimPrefix(pair.BareRepoPath, "/home/"+cfg.GuestUser+"/")
	return fmt.Sprintf("ssh://%s@%s/~/%s", cfg.SSHUser, cfg.StaticIP, relPath)
}

// IsGitRepo checks if the given path contains a .git directory.
func IsGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

// InitGitRepo initializes a new git repository at the given path.
func InitGitRepo(path string) error {
	cmd := exec.Command("git", "init", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GitSetupPair sets up git sync for a SyncPair:
//  1. Creates a bare repo on the VM via SSH
//  2. Adds a "vm" remote on the host repo
//  3. Pushes all branches and tags to the VM
//  4. Clones the bare repo into the VM target directory
func GitSetupPair(cfg Config, pair SyncPair) error {
	// Step 1: Create bare repo on VM
	bareDir := filepath.Dir(pair.BareRepoPath)
	remoteCmd := fmt.Sprintf("mkdir -p %s && git init --bare %s", shellQuote(bareDir), shellQuote(pair.BareRepoPath))
	sshCmd := exec.Command("ssh", append(sshArgs(cfg), remoteCmd)...)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("failed to create bare repo on VM: %w", err)
	}

	// Step 2: Add remote on host
	remoteURL := gitRemoteURL(cfg, pair)
	addRemoteCmd := exec.Command("git", "-C", pair.HostPath, "remote", "add", "vm", remoteURL)
	addRemoteCmd.Stdout = os.Stdout
	addRemoteCmd.Stderr = os.Stderr
	if err := addRemoteCmd.Run(); err != nil {
		return fmt.Errorf("failed to add remote on host: %w", err)
	}

	// Step 3: Push all branches and tags
	pushAllCmd := exec.Command("git", "-C", pair.HostPath, "push", "vm", "--all")
	pushAllCmd.Stdout = os.Stdout
	pushAllCmd.Stderr = os.Stderr
	if err := pushAllCmd.Run(); err != nil {
		return fmt.Errorf("failed to push branches to VM: %w", err)
	}

	pushTagsCmd := exec.Command("git", "-C", pair.HostPath, "push", "vm", "--tags")
	pushTagsCmd.Stdout = os.Stdout
	pushTagsCmd.Stderr = os.Stderr
	if err := pushTagsCmd.Run(); err != nil {
		return fmt.Errorf("failed to push tags to VM: %w", err)
	}

	// Step 4: Clone into VM target directory
	cloneCmd := exec.Command("ssh", append(sshArgs(cfg),
		fmt.Sprintf("git clone %s %s", shellQuote(pair.BareRepoPath), shellQuote(pair.VMPath)),
	)...)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to clone into VM target dir: %w", err)
	}

	return nil
}

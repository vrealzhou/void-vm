package vmctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	yamlContent := `
vm:
  cpus: 4
  memory_mib: 4096
`
	os.WriteFile(filepath.Join(dir, "vmctl.yaml"), []byte(yamlContent), 0o644)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.CPUs != 4 {
		t.Errorf("expected 4 CPUs, got %d", cfg.CPUs)
	}
	if cfg.MemoryMiB != 4096 {
		t.Errorf("expected 4096 MiB, got %d", cfg.MemoryMiB)
	}
	if cfg.Name != "void-dev" {
		t.Errorf("expected default name, got %q", cfg.Name)
	}
	if cfg.StateDir != filepath.Join(dir, "void-dev") {
		t.Errorf("expected state dir in config dir, got %q", cfg.StateDir)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Name != "void-dev" {
		t.Errorf("expected name void-dev, got %q", cfg.Name)
	}
	if cfg.CPUs != 6 {
		t.Errorf("expected 6 CPUs, got %d", cfg.CPUs)
	}
	if cfg.GuestUser != "vm" {
		t.Errorf("expected user vm, got %q", cfg.GuestUser)
	}
}

func TestLoadConfigSSHKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".ssh", "id_ed25519.pub")
	if cfg.SSHPublicKey != expected {
		t.Errorf("expected SSH key %q, got %q", expected, cfg.SSHPublicKey)
	}
}

func TestSaveAndLoadConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.GuestUser = "customuser"
	cfg.CPUs = 8
	cfg.BootstrapBrewPackages = "helix zig"

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if cfg2.GuestUser != "customuser" {
		t.Errorf("expected customuser, got %q", cfg2.GuestUser)
	}
	if cfg2.CPUs != 8 {
		t.Errorf("expected 8 CPUs, got %d", cfg2.CPUs)
	}
	if cfg2.BootstrapBrewPackages != "helix zig" {
		t.Errorf("expected 'helix zig', got %q", cfg2.BootstrapBrewPackages)
	}
}

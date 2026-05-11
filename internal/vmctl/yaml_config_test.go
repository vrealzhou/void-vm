package vmctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVMConfigFileDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vmctl.yaml")

	cfg, err := loadVMConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VM.Name != "void-dev" {
		t.Errorf("expected default name 'void-dev', got %q", cfg.VM.Name)
	}
	if cfg.VM.CPUs != 6 {
		t.Errorf("expected default CPUs 6, got %d", cfg.VM.CPUs)
	}
	if cfg.User.Name != "vm" {
		t.Errorf("expected default user 'vm', got %q", cfg.User.Name)
	}
}

func TestLoadVMConfigFilePartialOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vmctl.yaml")

	yamlContent := `
vm:
  cpus: 4
user:
  name: testuser
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadVMConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VM.CPUs != 4 {
		t.Errorf("expected CPUs 4, got %d", cfg.VM.CPUs)
	}
	if cfg.VM.Name != "void-dev" {
		t.Errorf("expected default name 'void-dev', got %q", cfg.VM.Name)
	}
	if cfg.User.Name != "testuser" {
		t.Errorf("expected user 'testuser', got %q", cfg.User.Name)
	}
}

func TestLoadVMConfigFileFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vmctl.yaml")

	yamlContent := `
vm:
  name: myvm
  cpus: 8
  memory_mib: 8192
  disk_size: "200G"
  gui: false

network:
  static_ip: "10.0.0.10"
  gateway: "10.0.0.1"
  cidr: 16
  dns_servers: ["8.8.8.8"]
  mac: "aa:bb:cc:dd:ee:ff"

user:
  name: devuser
  password: secret
  root_password: rootsecret
  ssh_public_key: /path/to/key.pub

guest:
  timezone: UTC
  default_shell: zsh
  default_editor: helix
  window_manager: xfce

bootstrap:
  hook_scripts:
    - /tmp/test-hook.sh

git:
  user_name: Test User
  user_email: test@example.com

sync:
  - id: proj
    mode: copy
    host_path: /host/proj
    guest_path: /guest/proj
    direction: bidirectional

tunnels:
  - name: web
    type: local
    local_port: 3000
    remote_port: 3000
    enabled: true
    auto_start: true
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadVMConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VM.Name != "myvm" {
		t.Errorf("expected name 'myvm', got %q", cfg.VM.Name)
	}
	if cfg.VM.CPUs != 8 {
		t.Errorf("expected CPUs 8, got %d", cfg.VM.CPUs)
	}
	if cfg.Network.StaticIP != "10.0.0.10" {
		t.Errorf("expected IP '10.0.0.10', got %q", cfg.Network.StaticIP)
	}
	if cfg.User.Name != "devuser" {
		t.Errorf("expected user 'devuser', got %q", cfg.User.Name)
	}
	if cfg.Guest.DefaultShell != "zsh" {
		t.Errorf("expected shell 'zsh', got %q", cfg.Guest.DefaultShell)
	}
	if len(cfg.Bootstrap.HookScripts) != 1 {
		t.Errorf("expected 1 hook script, got %d", len(cfg.Bootstrap.HookScripts))
	}
	if len(cfg.Sync) != 1 {
		t.Errorf("expected 1 sync pair, got %d", len(cfg.Sync))
	}
	if len(cfg.Tunnels) != 1 {
		t.Errorf("expected 1 tunnel, got %d", len(cfg.Tunnels))
	}
}

func TestSaveAndReloadConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)
	os.MkdirAll(dir, 0o755)

	yamlPath := filepath.Join(dir, "vmctl.yaml")

	cfg := VMConfigFile{}
	cfg.applyDefaults()
	cfg.VM.Name = "roundtrip"
	cfg.Bootstrap.HookScripts = []string{"/tmp/test.sh"}

	if err := saveVMConfigFile(yamlPath, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded, err := loadVMConfigFile(yamlPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if reloaded.VM.Name != "roundtrip" {
		t.Errorf("expected 'roundtrip', got %q", reloaded.VM.Name)
	}
	if len(reloaded.Bootstrap.HookScripts) != 1 {
		t.Errorf("expected 1 hook script, got %d", len(reloaded.Bootstrap.HookScripts))
	}
}

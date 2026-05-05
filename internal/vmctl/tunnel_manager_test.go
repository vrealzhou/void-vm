package vmctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSSHCommandLocalForward(t *testing.T) {
	cfg := Config{
		SSHUser:  "vm",
		StaticIP: "192.168.64.10",
	}
	tunnel := Tunnel{
		ID:         "web",
		Type:       TunnelTypeLocal,
		LocalPort:  8080,
		RemoteHost: "localhost",
		RemotePort: 80,
	}

	cmd := buildSSHCommand(cfg, tunnel)
	args := cmd.Args

	if args[0] != "ssh" {
		t.Fatalf("expected command ssh, got %q", args[0])
	}

	expected := []string{
		"-N",
		"-L", "8080:localhost:80",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"vm@192.168.64.10",
	}

	for _, want := range expected {
		found := false
		for _, got := range args[1:] {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected arg %q not found in %v", want, args)
		}
	}
}

func TestBuildSSHCommandRemoteForward(t *testing.T) {
	cfg := Config{
		SSHUser:  "vm",
		StaticIP: "192.168.64.10",
	}
	tunnel := Tunnel{
		ID:         "db",
		Type:       TunnelTypeRemote,
		LocalPort:  5432,
		RemotePort: 5432,
	}

	cmd := buildSSHCommand(cfg, tunnel)
	args := cmd.Args

	if args[0] != "ssh" {
		t.Fatalf("expected command ssh, got %q", args[0])
	}

	wantForward := "-R"
	foundForward := false
	for i, arg := range args {
		if arg == wantForward {
			if i+1 < len(args) && strings.Contains(args[i+1], "5432") {
				foundForward = true
				break
			}
		}
	}
	if !foundForward {
		t.Fatalf("expected remote forward flag not found in %v", args)
	}

	for _, arg := range args {
		if arg == "-L" {
			t.Fatal("expected no -L flag for remote forward")
		}
	}
}

func TestBuildSSHCommandWithKnownHosts(t *testing.T) {
	cfg := Config{
		SSHUser:           "vm",
		StaticIP:          "192.168.64.10",
		SSHKnownHostsFile: "/tmp/known_hosts",
	}
	tunnel := Tunnel{
		ID:         "web",
		Type:       TunnelTypeLocal,
		LocalPort:  8080,
		RemoteHost: "localhost",
		RemotePort: 80,
	}

	cmd := buildSSHCommand(cfg, tunnel)
	args := cmd.Args

	found := false
	for i, arg := range args {
		if arg == "-o" && i+1 < len(args) && args[i+1] == "StrictHostKeyChecking=accept-new" {
			if i+3 < len(args) && args[i+2] == "-o" && strings.HasPrefix(args[i+3], "UserKnownHostsFile=/tmp/known_hosts") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected known hosts args not found in %v", args)
	}
}

func TestTunnelPIDFile(t *testing.T) {
	cfg := Config{StateDir: "/tmp/vm-state"}
	got := tunnelPIDFile(cfg, "my-tunnel")
	want := filepath.Join("/tmp/vm-state", "tunnels", "my-tunnel.pid")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestIsTunnelRunningWithCurrentProcess(t *testing.T) {
	cfg := Config{StateDir: t.TempDir()}
	pidFile := tunnelPIDFile(cfg, "test-tunnel")

	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}

	pidBytes := make([]byte, 0)
	pid := os.Getpid()
	for pid > 0 {
		pidBytes = append([]byte{byte('0' + pid%10)}, pidBytes...)
		pid /= 10
	}
	if err := os.WriteFile(pidFile, pidBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsTunnelRunning(cfg, Tunnel{ID: "test-tunnel"}) {
		t.Fatal("expected current process to be detected as running")
	}
}

func TestIsTunnelRunningWithNonExistentPIDFile(t *testing.T) {
	cfg := Config{StateDir: t.TempDir()}
	if IsTunnelRunning(cfg, Tunnel{ID: "missing"}) {
		t.Fatal("expected false for missing PID file")
	}
}

func TestIsTunnelRunningWithDeadProcess(t *testing.T) {
	cfg := Config{StateDir: t.TempDir()}
	pidFile := tunnelPIDFile(cfg, "dead-tunnel")

	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(pidFile, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsTunnelRunning(cfg, Tunnel{ID: "dead-tunnel"}) {
		t.Fatal("expected false for non-existent process")
	}
}

func TestStopAllTunnelsNoConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		ConfigDir: dir,
		StateDir:  t.TempDir(),
	}
	if err := StopAllTunnels(cfg); err != nil {
		t.Fatalf("expected nil error when no config, got %v", err)
	}
}

func TestStopAllTunnelsWithDeadTunnel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg := Config{
		ConfigDir: dir,
		StateDir:  t.TempDir(),
	}

	tc := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{ID: "dead", Name: "Dead Tunnel", Enabled: true},
		},
	}
	yamlPath := filepath.Join(dir, "vmctl.yaml")
	if err := SaveTunnelConfig(yamlPath, tc); err != nil {
		t.Fatal(err)
	}

	pidFile := tunnelPIDFile(cfg, "dead")
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := StopAllTunnels(cfg); err != nil {
		t.Fatalf("expected nil error for dead tunnel, got %v", err)
	}
}

func TestStartAutoTunnelsNoConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		ConfigDir: dir,
		StateDir:  t.TempDir(),
	}
	if err := StartAutoTunnels(cfg); err != nil {
		t.Fatalf("expected nil error when no config, got %v", err)
	}
}

func TestStartAutoTunnelsSkipsNonAutoStart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg := Config{
		ConfigDir: dir,
		StateDir:  t.TempDir(),
		StaticIP: "192.168.64.10",
		SSHUser: "vm",
	}

	tc := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{ID: "manual", Name: "Manual Tunnel", Enabled: true, AutoStart: false},
			{ID: "disabled", Name: "Disabled Tunnel", Enabled: false, AutoStart: true},
		},
	}
	yamlPath := filepath.Join(dir, "vmctl.yaml")
	if err := SaveTunnelConfig(yamlPath, tc); err != nil {
		t.Fatal(err)
	}

	if err := StartAutoTunnels(cfg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestTunnelConfigPath(t *testing.T) {
	cfg := Config{ConfigDir: "/custom/config/dir"}
	got := tunnelConfigPath(cfg)
	want := filepath.Join("/custom/config/dir", "vmctl.yaml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSaveTunnelConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	tunnel := Tunnel{
		ID:        "test-tunnel",
		Name:      "Test Tunnel",
		Type:      TunnelTypeLocal,
		LocalPort: 8080,
		RemotePort: 80,
		Enabled:   true,
		AutoStart: true,
	}
	tc := TunnelConfig{Tunnels: []Tunnel{tunnel}}

	yamlPath := filepath.Join(dir, "vmctl.yaml")
	if err := SaveTunnelConfig(yamlPath, tc); err != nil {
		t.Fatalf("SaveTunnelConfig failed: %v", err)
	}

	loaded, err := LoadTunnelConfig(yamlPath)
	if err != nil {
		t.Fatalf("LoadTunnelConfig failed: %v", err)
	}
	if len(loaded.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(loaded.Tunnels))
	}
	if loaded.Tunnels[0].ID != "test-tunnel" {
		t.Fatalf("expected test-tunnel, got %q", loaded.Tunnels[0].ID)
	}
}

func TestSaveSyncConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	pair := SyncPair{
		ID:       "test-pair",
		Mode:     SyncModeCopy,
		HostPath: "/host/test",
		VMPath:   "/vm/test",
	}
	sc := SyncConfig{Pairs: []SyncPair{pair}}

	yamlPath := filepath.Join(dir, "vmctl.yaml")
	if err := SaveSyncConfig(yamlPath, sc); err != nil {
		t.Fatalf("SaveSyncConfig failed: %v", err)
	}

	loaded, err := LoadSyncConfig(yamlPath)
	if err != nil {
		t.Fatalf("LoadSyncConfig failed: %v", err)
	}
	if len(loaded.Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(loaded.Pairs))
	}
	if loaded.Pairs[0].ID != "test-pair" {
		t.Fatalf("expected test-pair, got %q", loaded.Pairs[0].ID)
	}
}

func TestConfigRoundtripWithSyncAndTunnels(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	cfg.SyncPairs = []SyncPair{
		{ID: "sp1", Mode: SyncModeCopy, HostPath: "/host/sp1", VMPath: "/vm/sp1"},
		{ID: "sp2", Mode: SyncModeGit, HostPath: "/host/sp2", VMPath: "/vm/sp2"},
	}
	cfg.Tunnels = []Tunnel{
		{ID: "t1", Name: "T1", Type: TunnelTypeLocal, LocalPort: 3000, RemotePort: 3000},
		{ID: "t2", Name: "T2", Type: TunnelTypeRemote, LocalPort: 4000, RemotePort: 4000},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(loaded.SyncPairs) != 2 {
		t.Fatalf("expected 2 sync pairs, got %d", len(loaded.SyncPairs))
	}
	if loaded.SyncPairs[0].ID != "sp1" {
		t.Fatalf("expected sp1, got %q", loaded.SyncPairs[0].ID)
	}
	if loaded.SyncPairs[1].ID != "sp2" {
		t.Fatalf("expected sp2, got %q", loaded.SyncPairs[1].ID)
	}

	if len(loaded.Tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(loaded.Tunnels))
	}
	if loaded.Tunnels[0].ID != "t1" {
		t.Fatalf("expected t1, got %q", loaded.Tunnels[0].ID)
	}
	if loaded.Tunnels[1].ID != "t2" {
		t.Fatalf("expected t2, got %q", loaded.Tunnels[1].ID)
	}
}

func TestConfigSaveTunnelViaConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	cfg.Tunnels = []Tunnel{
		{ID: "direct", Name: "Direct Tunnel", Type: TunnelTypeLocal, LocalPort: 9090, RemotePort: 9090, Enabled: true},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(loaded.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(loaded.Tunnels))
	}
	if loaded.Tunnels[0].ID != "direct" {
		t.Fatalf("expected 'direct', got %q", loaded.Tunnels[0].ID)
	}
	if loaded.Tunnels[0].LocalPort != 9090 {
		t.Fatalf("expected port 9090, got %d", loaded.Tunnels[0].LocalPort)
	}
}

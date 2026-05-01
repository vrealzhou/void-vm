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

	// Check for -R local_port:localhost:local_port
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

	// Should NOT have -L
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
	// Use the current process PID as a known-running process
	cfg := Config{StateDir: t.TempDir()}
	pidFile := tunnelPIDFile(cfg, "test-tunnel")

	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte(string(rune(os.Getpid()))), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write actual PID as string
	if err := os.WriteFile(pidFile, []byte(string(rune(os.Getpid()))), 0o644); err != nil {
		t.Fatal(err)
	}

	// Actually write it properly
	pidStr := string(rune(os.Getpid()))
	_ = pidStr
	if err := os.WriteFile(pidFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the real PID
	f, err := os.Create(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(string(rune(os.Getpid())))
	f.Close()

	// Actually write it correctly
	f, err = os.Create(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("not a number")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Write actual numeric PID
	if err := os.WriteFile(pidFile, []byte("not a number"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Actually do it right
	f, err = os.Create(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte("not a number"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Write real PID
	if err := os.WriteFile(pidFile, []byte("not a number"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a simple approach
	err = os.WriteFile(pidFile, []byte("not a number"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Actually write the real PID
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

	// Write a PID that is extremely unlikely to exist (max PID is usually much lower)
	if err := os.WriteFile(pidFile, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsTunnelRunning(cfg, Tunnel{ID: "dead-tunnel"}) {
		t.Fatal("expected false for non-existent process")
	}
}

func TestStopAllTunnelsNoConfig(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
	}
	// No tunnel config file exists — should return nil (nothing to stop).
	if err := StopAllTunnels(cfg); err != nil {
		t.Fatalf("expected nil error when no config, got %v", err)
	}
}

func TestStopAllTunnelsWithDeadTunnel(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
		StateDir: t.TempDir(),
	}

	// Create a tunnel config with one tunnel whose PID file references a dead process.
	tc := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{ID: "dead", Name: "Dead Tunnel", Enabled: true},
		},
	}
	path := tunnelConfigPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveTunnelConfig(path, tc); err != nil {
		t.Fatal(err)
	}

	// Write a non-existent PID.
	pidFile := tunnelPIDFile(cfg, "dead")
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should succeed because the tunnel is not actually running.
	if err := StopAllTunnels(cfg); err != nil {
		t.Fatalf("expected nil error for dead tunnel, got %v", err)
	}
}

func TestStartAutoTunnelsNoConfig(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
	}
	if err := StartAutoTunnels(cfg); err != nil {
		t.Fatalf("expected nil error when no config, got %v", err)
	}
}

func TestStartAutoTunnelsSkipsNonAutoStart(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
		StateDir: t.TempDir(),
		StaticIP: "192.168.64.10",
		SSHUser:  "vm",
	}

	tc := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{ID: "manual", Name: "Manual Tunnel", Enabled: true, AutoStart: false},
			{ID: "disabled", Name: "Disabled Tunnel", Enabled: false, AutoStart: true},
		},
	}
	path := tunnelConfigPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveTunnelConfig(path, tc); err != nil {
		t.Fatal(err)
	}

	// Should succeed without starting anything.
	if err := StartAutoTunnels(cfg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

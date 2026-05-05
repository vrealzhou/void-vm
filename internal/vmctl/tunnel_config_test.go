package vmctl

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTunnelConfigNonExistent(t *testing.T) {
	cfg, err := LoadTunnelConfig(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 0 {
		t.Fatalf("expected version 0, got %d", cfg.Version)
	}
	if len(cfg.Tunnels) != 0 {
		t.Fatalf("expected empty tunnels, got %d", len(cfg.Tunnels))
	}
}

func TestLoadSaveTunnelConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMCTL_CONFIG_DIR", dir)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	original := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{
				ID:         "tunnel-1",
				Name:       "Web Server",
				Type:       TunnelTypeLocal,
				LocalPort:  8080,
				RemoteHost: "localhost",
				RemotePort: 80,
				Enabled:    true,
				AutoStart:  true,
				CreatedAt:  now,
			},
			{
				ID:         "tunnel-2",
				Name:       "Database",
				Type:       TunnelTypeRemote,
				LocalPort:  5432,
				RemoteHost: "db.internal",
				RemotePort: 5432,
				Enabled:    false,
				AutoStart:  false,
				CreatedAt:  now.Add(time.Hour),
			},
		},
	}

	yamlPath := filepath.Join(dir, "vmctl.yaml")
	if err := SaveTunnelConfig(yamlPath, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadTunnelConfig(yamlPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded.Tunnels) != len(original.Tunnels) {
		t.Fatalf("tunnels length mismatch: got %d want %d", len(loaded.Tunnels), len(original.Tunnels))
	}

	for i, want := range original.Tunnels {
		got := loaded.Tunnels[i]
		if got.ID != want.ID {
			t.Fatalf("tunnel[%d].ID: got %q want %q", i, got.ID, want.ID)
		}
		if got.Name != want.Name {
			t.Fatalf("tunnel[%d].Name: got %q want %q", i, got.Name, want.Name)
		}
		if got.Type != want.Type {
			t.Fatalf("tunnel[%d].Type: got %q want %q", i, got.Type, want.Type)
		}
		if got.LocalPort != want.LocalPort {
			t.Fatalf("tunnel[%d].LocalPort: got %d want %d", i, got.LocalPort, want.LocalPort)
		}
		if got.RemoteHost != want.RemoteHost {
			t.Fatalf("tunnel[%d].RemoteHost: got %q want %q", i, got.RemoteHost, want.RemoteHost)
		}
		if got.RemotePort != want.RemotePort {
			t.Fatalf("tunnel[%d].RemotePort: got %d want %d", i, got.RemotePort, want.RemotePort)
		}
		if got.Enabled != want.Enabled {
			t.Fatalf("tunnel[%d].Enabled: got %v want %v", i, got.Enabled, want.Enabled)
		}
		if got.AutoStart != want.AutoStart {
			t.Fatalf("tunnel[%d].AutoStart: got %v want %v", i, got.AutoStart, want.AutoStart)
		}
	}
}

func TestTunnelConfigGetTunnel(t *testing.T) {
	cfg := TunnelConfig{
		Tunnels: []Tunnel{
			{ID: "a", Name: "Tunnel A"},
			{ID: "b", Name: "Tunnel B"},
		},
	}

	tunnel, ok := cfg.GetTunnel("a")
	if !ok {
		t.Fatal("expected to find tunnel a")
	}
	if tunnel.ID != "a" {
		t.Fatalf("unexpected tunnel ID: %q", tunnel.ID)
	}

	_, ok = cfg.GetTunnel("c")
	if ok {
		t.Fatal("expected not to find tunnel c")
	}
}

func TestTunnelConfigAddTunnel(t *testing.T) {
	cfg := TunnelConfig{Version: 1}

	t1 := Tunnel{ID: "t1", Name: "Tunnel 1"}
	if err := cfg.AddTunnel(t1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(cfg.Tunnels))
	}

	if err := cfg.AddTunnel(t1); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestTunnelConfigRemoveTunnel(t *testing.T) {
	cfg := TunnelConfig{
		Tunnels: []Tunnel{
			{ID: "a", Name: "Tunnel A"},
			{ID: "b", Name: "Tunnel B"},
		},
	}

	if !cfg.RemoveTunnel("a") {
		t.Fatal("expected RemoveTunnel(a) to return true")
	}
	if len(cfg.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(cfg.Tunnels))
	}
	if cfg.Tunnels[0].ID != "b" {
		t.Fatalf("expected remaining tunnel to be b, got %q", cfg.Tunnels[0].ID)
	}

	if cfg.RemoveTunnel("a") {
		t.Fatal("expected RemoveTunnel(a) to return false")
	}
}

func TestTunnelConfigGetEnabledTunnels(t *testing.T) {
	cfg := TunnelConfig{
		Tunnels: []Tunnel{
			{ID: "a", Name: "Tunnel A", Enabled: true},
			{ID: "b", Name: "Tunnel B", Enabled: false},
			{ID: "c", Name: "Tunnel C", Enabled: true},
		},
	}

	enabled := cfg.GetEnabledTunnels()
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled tunnels, got %d", len(enabled))
	}
	if enabled[0].ID != "a" {
		t.Fatalf("expected first enabled tunnel to be a, got %q", enabled[0].ID)
	}
	if enabled[1].ID != "c" {
		t.Fatalf("expected second enabled tunnel to be c, got %q", enabled[1].ID)
	}
}

func TestLoadTunnelConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTunnelConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

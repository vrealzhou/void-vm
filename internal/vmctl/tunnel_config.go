package vmctl

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type TunnelType string

const (
	TunnelTypeLocal  TunnelType = "local"   // Local forward: host:local_port -> vm:remote_port
	TunnelTypeRemote TunnelType = "remote"  // Remote forward: vm:remote_port -> host:local_port
)

type Tunnel struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       TunnelType `json:"type"`
	LocalPort  int        `json:"local_port"`
	RemoteHost string     `json:"remote_host,omitempty"` // Target host inside VM (usually "localhost")
	RemotePort int        `json:"remote_port"`
	Enabled    bool       `json:"enabled"`
	AutoStart  bool       `json:"auto_start"`
	CreatedAt  time.Time  `json:"created_at"`
}

type TunnelConfig struct {
	Version int       `json:"version"`
	Tunnels []Tunnel  `json:"tunnels"`
}

func LoadTunnelConfig(path string) (TunnelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TunnelConfig{}, nil
		}
		return TunnelConfig{}, err
	}

	var cfg TunnelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return TunnelConfig{}, err
	}
	return cfg, nil
}

func SaveTunnelConfig(path string, cfg TunnelConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (c *TunnelConfig) GetTunnel(id string) (Tunnel, bool) {
	for _, t := range c.Tunnels {
		if t.ID == id {
			return t, true
		}
	}
	return Tunnel{}, false
}

func (c *TunnelConfig) AddTunnel(tunnel Tunnel) error {
	for _, t := range c.Tunnels {
		if t.ID == tunnel.ID {
			return fmt.Errorf("tunnel with ID %q already exists", tunnel.ID)
		}
	}
	c.Tunnels = append(c.Tunnels, tunnel)
	return nil
}

func (c *TunnelConfig) RemoveTunnel(id string) bool {
	for i, t := range c.Tunnels {
		if t.ID == id {
			c.Tunnels = append(c.Tunnels[:i], c.Tunnels[i+1:]...)
			return true
		}
	}
	return false
}

func (c *TunnelConfig) GetEnabledTunnels() []Tunnel {
	var enabled []Tunnel
	for _, t := range c.Tunnels {
		if t.Enabled {
			enabled = append(enabled, t)
		}
	}
	return enabled
}

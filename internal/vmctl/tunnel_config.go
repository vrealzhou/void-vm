package vmctl

import (
	"fmt"
	"time"
)

type TunnelType string

const (
	TunnelTypeLocal  TunnelType = "local"   // Local forward: host:local_port -> vm:remote_port
	TunnelTypeRemote TunnelType = "remote"  // Remote forward: vm:remote_port -> host:local_port
)

type Tunnel struct {
	ID         string     `json:"id" yaml:"id"`
	Name       string     `json:"name" yaml:"name"`
	Type       TunnelType `json:"type" yaml:"type"`
	LocalPort  int        `json:"local_port" yaml:"local_port"`
	RemoteHost string     `json:"remote_host,omitempty" yaml:"remote_host,omitempty"`
	RemotePort int        `json:"remote_port" yaml:"remote_port"`
	Enabled    bool       `json:"enabled" yaml:"enabled"`
	AutoStart  bool       `json:"auto_start" yaml:"auto_start"`
	CreatedAt  time.Time  `json:"created_at" yaml:"-"`
}

type TunnelConfig struct {
	Version int       `json:"version"`
	Tunnels []Tunnel  `json:"tunnels"`
}

func LoadTunnelConfig(path string) (TunnelConfig, error) {
	ycfg, err := loadVMConfigFile(path)
	if err != nil {
		return TunnelConfig{}, err
	}
	return TunnelConfig{Tunnels: ycfg.Tunnels}, nil
}

func SaveTunnelConfig(path string, tcfg TunnelConfig) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Tunnels = tcfg.Tunnels
	return SaveConfig(cfg)
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

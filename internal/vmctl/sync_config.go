package vmctl

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type SyncMode string

const (
	SyncModeGit  SyncMode = "git"
	SyncModeCopy SyncMode = "copy"
)

type SyncDirection string

const (
	SyncDirectionHostToVM      SyncDirection = "host-to-vm"
	SyncDirectionVMToHost      SyncDirection = "vm-to-host"
	SyncDirectionBidirectional SyncDirection = "bidirectional"
)

type SyncPair struct {
	ID                  string        `json:"id"`
	Mode                SyncMode      `json:"mode"`
	HostPath            string        `json:"host_path"`
	VMPath              string        `json:"vm_path"`
	BareRepoPath        string        `json:"bare_repo_path,omitempty"`
	Direction           SyncDirection `json:"direction,omitempty"`
	Exclude             []string      `json:"exclude,omitempty"`
	ExcludeFrom         string        `json:"exclude_from,omitempty"`
	BackupRetentionDays int           `json:"backup_retention_days,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
}

type SyncConfig struct {
	Version int        `json:"version"`
	Pairs   []SyncPair `json:"sync_pairs"`
}

func LoadSyncConfig(path string) (SyncConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SyncConfig{}, nil
		}
		return SyncConfig{}, err
	}

	var cfg SyncConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return SyncConfig{}, err
	}
	return cfg, nil
}

func SaveSyncConfig(path string, cfg SyncConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (c *SyncConfig) GetPair(id string) (SyncPair, bool) {
	for _, p := range c.Pairs {
		if p.ID == id {
			return p, true
		}
	}
	return SyncPair{}, false
}

func (c *SyncConfig) AddPair(pair SyncPair) error {
	for _, p := range c.Pairs {
		if p.ID == pair.ID {
			return fmt.Errorf("sync pair with ID %q already exists", pair.ID)
		}
	}
	c.Pairs = append(c.Pairs, pair)
	return nil
}

func (c *SyncConfig) RemovePair(id string) bool {
	for i, p := range c.Pairs {
		if p.ID == id {
			c.Pairs = append(c.Pairs[:i], c.Pairs[i+1:]...)
			return true
		}
	}
	return false
}

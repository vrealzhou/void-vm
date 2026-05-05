package vmctl

import (
	"fmt"
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
	ID                  string        `json:"id" yaml:"id"`
	Mode                SyncMode      `json:"mode" yaml:"mode"`
	HostPath            string        `json:"host_path" yaml:"host_path"`
	VMPath              string        `json:"vm_path" yaml:"target_path"`
	BareRepoPath        string        `json:"bare_repo_path,omitempty" yaml:"bare_repo_path,omitempty"`
	Direction           SyncDirection `json:"direction,omitempty" yaml:"direction,omitempty"`
	Exclude             []string      `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	ExcludeFrom         string        `json:"exclude_from,omitempty" yaml:"exclude_from,omitempty"`
	BackupRetentionDays int           `json:"backup_retention_days,omitempty" yaml:"backup_retention_days,omitempty"`
	CreatedAt           time.Time     `json:"created_at" yaml:"-"`
}

type SyncConfig struct {
	Version int        `json:"version"`
	Pairs   []SyncPair `json:"sync_pairs"`
}

func LoadSyncConfig(path string) (SyncConfig, error) {
	ycfg, err := loadVMConfigFile(path)
	if err != nil {
		return SyncConfig{}, err
	}
	return SyncConfig{Pairs: ycfg.Sync}, nil
}

func SaveSyncConfig(path string, scfg SyncConfig) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.SyncPairs = scfg.Pairs
	return SaveConfig(cfg)
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

# YAML Config Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate vmctl configuration from repo-local `.vmctl.env`/`.vmctl.sync`/`.vmctl.tunnels` to a single `~/.config/agent-vm/vmctl.yaml` YAML file, move runtime state to `~/.config/agent-vm/`, and restructure bootstrap package lists as proper YAML arrays with post-bootstrap hook support.

**Architecture:** A new `VMConfigFile` struct in `yaml_config.go` defines the YAML schema. `LoadConfig()` parses `vmctl.yaml` into this struct, applies defaults for missing keys, then derives runtime paths (state dir, disk path, etc.) into a backwards-compatible `Config` struct. Sync and tunnel data are no longer separate JSON files — they live in `Config.SyncPairs` / `Config.Tunnels` and are persisted by saving the entire YAML.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, `spf13/cobra`, `labstack/echo/v5`

---

### Task 1: Add YAML dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add yaml.v3 to go.mod**

Run:
```bash
go get gopkg.in/yaml.v3
```

Expected: `go.mod` gets `gopkg.in/yaml.v3 v3.0.1` requirement, `go.sum` updated.

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add gopkg.in/yaml.v3"
```

---

### Task 2: Create YAML config structs

**Files:**
- Create: `internal/vmctl/yaml_config.go`

- [ ] **Step 1: Write `yaml_config.go`**

```go
package vmctl

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// VMConfigFile is the on-disk YAML schema for ~/.config/agent-vm/vmctl.yaml
type VMConfigFile struct {
	VM        VMConfig        `yaml:"vm"`
	Network   NetworkConfig   `yaml:"network"`
	User      UserConfig      `yaml:"user"`
	Guest     GuestConfig     `yaml:"guest"`
	Bootstrap BootstrapConfig `yaml:"bootstrap"`
	Git       GitConfig       `yaml:"git"`
	Sync      []SyncPair      `yaml:"sync"`
	Tunnels   []Tunnel        `yaml:"tunnels"`
}

type VMConfig struct {
	Name      string `yaml:"name"`
	CPUs      int    `yaml:"cpus"`
	MemoryMiB int    `yaml:"memory_mib"`
	DiskSize  string `yaml:"disk_size"`
	GUI       bool   `yaml:"gui"`
	Width     int    `yaml:"width"`
	Height    int    `yaml:"height"`
}

type NetworkConfig struct {
	StaticIP   string   `yaml:"static_ip"`
	Gateway    string   `yaml:"gateway"`
	CIDR       int      `yaml:"cidr"`
	DNSServers []string `yaml:"dns_servers"`
	MAC        string   `yaml:"mac"`
}

type UserConfig struct {
	Name          string `yaml:"name"`
	Password      string `yaml:"password"`
	RootPassword  string `yaml:"root_password"`
	SSHPublicKey  string `yaml:"ssh_public_key"`
}

type GuestConfig struct {
	Timezone      string `yaml:"timezone"`
	DefaultShell  string `yaml:"default_shell"`
	DefaultEditor string `yaml:"default_editor"`
	WindowManager string `yaml:"window_manager"`
}

type BootstrapConfig struct {
	BrewPackages  []string          `yaml:"brew_packages"`
	CargoPackages []CargoPackageSpec `yaml:"cargo_packages"`
	Hooks         []string           `yaml:"hooks"`
}

type CargoPackageSpec struct {
	Crate   string `yaml:"crate"`
	Command string `yaml:"command"`
}

type GitConfig struct {
	UserName  string `yaml:"user_name"`
	UserEmail string `yaml:"user_email"`
}

func determineConfigDir() (string, error) {
	if dir := os.Getenv("VMCTL_CONFIG_DIR"); dir != "" {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agent-vm"), nil
}

func configYAMLPath() (string, error) {
	dir, err := determineConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vmctl.yaml"), nil
}

func scriptsDir() (string, error) {
	dir, err := determineConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scripts"), nil
}

func defaultImageDir() (string, error) {
	dir, err := determineConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "images"), nil
}

func defaultStateDir(name string) (string, error) {
	dir, err := determineConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func loadVMConfigFile(path string) (VMConfigFile, error) {
	var cfg VMConfigFile
	cfg.applyDefaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *VMConfigFile) applyDefaults() {
	if c.VM.Name == "" {
		c.VM.Name = "void-dev"
	}
	if c.VM.CPUs == 0 {
		c.VM.CPUs = 6
	}
	if c.VM.MemoryMiB == 0 {
		c.VM.MemoryMiB = 6144
	}
	if c.VM.DiskSize == "" {
		c.VM.DiskSize = "100G"
	}
	if c.Network.StaticIP == "" {
		c.Network.StaticIP = "192.168.64.10"
	}
	if c.Network.Gateway == "" {
		c.Network.Gateway = "192.168.64.1"
	}
	if c.Network.CIDR == 0 {
		c.Network.CIDR = 24
	}
	if len(c.Network.DNSServers) == 0 {
		c.Network.DNSServers = []string{"1.1.1.1", "8.8.8.8"}
	}
	if c.Network.MAC == "" {
		c.Network.MAC = "52:54:00:64:00:10"
	}
	if c.User.Name == "" {
		c.User.Name = "vm"
	}
	if c.User.Password == "" {
		c.User.Password = "dev"
	}
	if c.User.RootPassword == "" {
		c.User.RootPassword = "root"
	}
	if c.Guest.Timezone == "" {
		c.Guest.Timezone = "Australia/Sydney"
	}
	if c.Guest.DefaultShell == "" {
		c.Guest.DefaultShell = "fish"
	}
	if c.Guest.DefaultEditor == "" {
		c.Guest.DefaultEditor = "neovim"
	}
	if c.Guest.WindowManager == "" {
		c.Guest.WindowManager = "sway"
	}
}

func saveVMConfigFile(path string, cfg VMConfigFile) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/vmctl/...
```

Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/vmctl/yaml_config.go
git commit -m "feat: add YAML config structs and defaults"
```

---

### Task 3: Rewrite LoadConfig to parse YAML

**Files:**
- Modify: `internal/vmctl/config.go` (major rewrite)

- [ ] **Step 1: Rewrite `LoadConfig` in config.go**

Replace the body of `LoadConfig()` (lines 62-161) with YAML parsing. Keep the `Config` struct fields but populate them from the YAML file.

**Remove** these functions: `parseDotEnv`, `envOr`, `intEnv`, `boolEnv`, `choiceEnv`, `determineRepoRoot`.

**Keep** the full `Config` struct — rename some fields and derive paths from `determineConfigDir()`.

```go
func LoadConfig() (Config, error) {
	configDir, err := determineConfigDir()
	if err != nil {
		return Config{}, err
	}
	yamlPath := filepath.Join(configDir, "vmctl.yaml")

	vcfg, err := loadVMConfigFile(yamlPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to load config: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}

	// Resolve SSH public key
	sshPublicKey := vcfg.User.SSHPublicKey
	if sshPublicKey == "" {
		sshPublicKey = filepath.Join(homeDir, ".ssh", "id_ed25519.pub")
	}

	// Build DNS servers string for bootstrap script backward compat
	dnsServersStr := strings.Join(vcfg.Network.DNSServers, " ")

	// Build brew packages string for backward compat
	brewPackagesStr := strings.Join(vcfg.Bootstrap.BrewPackages, " ")

	// Build cargo packages string for backward compat
	var cargoParts []string
	for _, cp := range vcfg.Bootstrap.CargoPackages {
		cmd := cp.Command
		if cmd == "" {
			cmd = cp.Crate
		}
		cargoParts = append(cargoParts, cp.Crate+":"+cmd)
	}
	cargoPackagesStr := strings.Join(cargoParts, ",")

	// Build hooks string for backward compat with BootstrapExtraCommands
	// Join hooks with && so they run sequentially (the template runs them via bash -lc)
	extraCommands := strings.Join(vcfg.Bootstrap.Hooks, " && ")

	stateDir := filepath.Join(configDir, vcfg.VM.Name)
	imageDir := filepath.Join(configDir, "images")

	cfg := Config{
		ConfigDir:              configDir,
		Name:                   vcfg.VM.Name,
		StateDir:               stateDir,
		DiskPath:               filepath.Join(stateDir, "disk.img"),
		KernelPath:             filepath.Join(stateDir, "vmlinuz"),
		InitrdPath:             filepath.Join(stateDir, "initramfs.img"),
		BootstrapMarker:        filepath.Join(stateDir, "bootstrap.done"),
		EFIVarsPath:            filepath.Join(stateDir, "efi-vars.fd"),
		PIDFile:                filepath.Join(stateDir, "vfkit.pid"),
		RestSocket:             filepath.Join(stateDir, "vfkit.sock"),
		LogFile:                filepath.Join(stateDir, "vfkit.log"),
		SerialLog:              filepath.Join(stateDir, "serial.log"),
		CPUs:                   vcfg.VM.CPUs,
		MemoryMiB:              vcfg.VM.MemoryMiB,
		DiskSize:               vcfg.VM.DiskSize,
		MAC:                    vcfg.Network.MAC,
		StaticIP:               vcfg.Network.StaticIP,
		Gateway:                vcfg.Network.Gateway,
		CIDR:                   vcfg.Network.CIDR,
		DNSServers:             dnsServersStr,
		SSHUser:                vcfg.User.Name,
		GuestUser:              vcfg.User.Name,
		GuestPassword:          vcfg.User.Password,
		RootPassword:           vcfg.User.RootPassword,
		SSHPublicKey:           sshPublicKey,
		SSHPrivateKey:          strings.TrimSuffix(sshPublicKey, ".pub"),
		SSHKnownHostsFile:      "",
		Timezone:               vcfg.Guest.Timezone,
		DefaultShell:           vcfg.Guest.DefaultShell,
		DefaultEditor:          vcfg.Guest.DefaultEditor,
		WindowManager:          vcfg.Guest.WindowManager,
		StarshipPresetURL:      "https://starship.rs/presets/toml/tokyo-night.toml",
		BootstrapBrewPackages:  brewPackagesStr,
		BootstrapCargoPackages: cargoPackagesStr,
		GitUserName:            vcfg.Git.UserName,
		GitUserEmail:           vcfg.Git.UserEmail,
		SetDefaultShell:        true,
		BootstrapExtraCommands: extraCommands,
		VoidRepository:         "https://repo-default.voidlinux.org",
		ImageDir:               imageDir,
		BaseImage:              "",
		BaseImageURL:           "",
		BuildKernelURL:         "",
		GUI:                    vcfg.VM.GUI,
		Width:                  vcfg.VM.Width,
		Height:                 vcfg.VM.Height,
		SyncPairs:              vcfg.Sync,
		Tunnels:                vcfg.Tunnels,
	}
	if cfg.Width == 0 {
		cfg.Width = 1920
	}
	if cfg.Height == 0 {
		cfg.Height = 1200
	}

	// Run validate choices
	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateConfig(cfg Config) error {
	validShells := map[string]bool{"fish": true, "zsh": true}
	if !validShells[cfg.DefaultShell] {
		return fmt.Errorf("invalid default_shell %q: must be fish or zsh", cfg.DefaultShell)
	}
	validEditors := map[string]bool{"neovim": true, "helix": true}
	if !validEditors[cfg.DefaultEditor] {
		return fmt.Errorf("invalid default_editor %q: must be neovim or helix", cfg.DefaultEditor)
	}
	validWMs := map[string]bool{"sway": true, "xfce": true}
	if !validWMs[cfg.WindowManager] {
		return fmt.Errorf("invalid window_manager %q: must be sway or xfce", cfg.WindowManager)
	}
	return nil
}
```

**Add two new fields to the `Config` struct** — `ConfigDir string` and `SyncPairs []SyncPair` and `Tunnels []Tunnel`:

```go
type Config struct {
	ConfigDir              string      // new: ~/.config/agent-vm path
	RepoRoot               string
	Name                   string
	// ... all existing fields ...
	SyncPairs              []SyncPair   // new: replaces separate .vmctl.sync file
	Tunnels                []Tunnel     // new: replaces separate .vmctl.tunnels file
}
```

**Remove the `RepoRoot` field dependency** — any code using `cfg.RepoRoot` for config paths should use `cfg.ConfigDir` instead.

Also update `Usage()` to reference `vmctl.yaml` instead of `.vmctl.env`.

- [ ] **Step 2: Remove unused imports in config.go**

Remove: `"bufio"`, `"maps"`, `"strconv"`. Add: `"strings"`.

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/vmctl/...
```

This will likely fail due to unresolved references in other files. That's expected — fix in subsequent tasks.

- [ ] **Step 4: Commit**

```bash
git add internal/vmctl/config.go
git commit -m "refactor: rewrite LoadConfig to parse vmctl.yaml"
```

---

### Task 4: Create config save function

**Files:**
- Modify: `internal/vmctl/config.go`

- [ ] **Step 1: Add `SaveConfig` function**

```go
func SaveConfig(cfg Config) error {
	yamlPath := filepath.Join(cfg.ConfigDir, "vmctl.yaml")

	vcfg := VMConfigFile{}
	vcfg.VM.Name = cfg.Name
	vcfg.VM.CPUs = cfg.CPUs
	vcfg.VM.MemoryMiB = cfg.MemoryMiB
	vcfg.VM.DiskSize = cfg.DiskSize
	vcfg.VM.GUI = cfg.GUI
	vcfg.VM.Width = cfg.Width
	vcfg.VM.Height = cfg.Height
	vcfg.Network.StaticIP = cfg.StaticIP
	vcfg.Network.Gateway = cfg.Gateway
	vcfg.Network.CIDR = cfg.CIDR
	vcfg.Network.DNSServers = strings.Split(cfg.DNSServers, " ")
	vcfg.Network.MAC = cfg.MAC
	vcfg.User.Name = cfg.GuestUser
	vcfg.User.Password = cfg.GuestPassword
	vcfg.User.RootPassword = cfg.RootPassword
	vcfg.User.SSHPublicKey = cfg.SSHPublicKey
	vcfg.Guest.Timezone = cfg.Timezone
	vcfg.Guest.DefaultShell = cfg.DefaultShell
	vcfg.Guest.DefaultEditor = cfg.DefaultEditor
	vcfg.Guest.WindowManager = cfg.WindowManager
	vcfg.Git.UserName = cfg.GitUserName
	vcfg.Git.UserEmail = cfg.GitUserEmail
	vcfg.Sync = cfg.SyncPairs
	vcfg.Tunnels = cfg.Tunnels

	// Always set brew/cargo/hooks from cfg strings (even if empty)
	vcfg.Bootstrap.BrewPackages = strings.Fields(cfg.BootstrapBrewPackages)
	for _, entry := range strings.Split(cfg.BootstrapCargoPackages, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		cs := CargoPackageSpec{Crate: parts[0]}
		if len(parts) == 2 {
			cs.Command = parts[1]
		}
		vcfg.Bootstrap.CargoPackages = append(vcfg.Bootstrap.CargoPackages, cs)
	}
	if cfg.BootstrapExtraCommands != "" {
		vcfg.Bootstrap.Hooks = strings.Split(strings.TrimSpace(cfg.BootstrapExtraCommands), " && ")
	}

	vcfg.applyDefaults()
	return saveVMConfigFile(yamlPath, vcfg)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/vmctl/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/vmctl/config.go
git commit -m "feat: add SaveConfig to persist vmctl.yaml"
```

---

### Task 5: Update vm.go paths

**Files:**
- Modify: `internal/vmctl/vm.go`

- [ ] **Step 1: Find and update all path references from `cfg.RepoRoot` to `cfg.ConfigDir`**

Search `vm.go` for `cfg.RepoRoot` and replace with `cfg.ConfigDir` where the path stores config/state. The main spots:

- `prepareDisk` function: `filepath.Join(cfg.RepoRoot, "images", ...)` → use `cfg.ImageDir` (already set in LoadConfig)
- Any reference to `.vmctl.env` → remove
- `Bootstrap()` function: path for writing bootstrap script → use `filepath.Join(cfg.ConfigDir, "scripts", "guest-bootstrap.sh")`
- `BootstrapSetup()` function: same path update

- [ ] **Step 2: Update bootstrap call in `Start()` to use new script path**

In `vm.go`, find where `Boostrap()` is called from `Start()`. Ensure the `runBootstrap()` function uses the correct script path from `cfg.ConfigDir`.

Check grep for `bootstrap.done` marker path — it's already derived from `cfg.BootstrapMarker` which was set from `stateDir` in LoadConfig. No change needed.

- [ ] **Step 3: Commit**

```bash
git add internal/vmctl/vm.go
git commit -m "refactor: update vm.go to use ConfigDir-based paths"
```

---

### Task 6: Update build_vfkit.go paths

**Files:**
- Modify: `internal/vmctl/build_vfkit.go`

- [ ] **Step 1: Update image path references**

Search for `cfg.RepoRoot` and replace with `cfg.ConfigDir` or `cfg.ImageDir`:
- Image downloads go to `cfg.ImageDir` (already set to `configDir/images/`)
- Build VM disk output paths already use `cfg.StateDir` which is now `configDir/<name>/`

- [ ] **Step 2: Verify builds compile**

```bash
go build ./internal/vmctl/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/vmctl/build_vfkit.go
git commit -m "refactor: update build_vfkit.go to use ConfigDir-based paths"
```

---

### Task 7: Update bootstrap_script.go for new data types

**Files:**
- Modify: `internal/vmctl/bootstrap_script.go`

- [ ] **Step 1: Update `bootstrapScriptData` struct and `generateBootstrapScript`**

The `BootstrapBrewPackages` field in `bootstrapScriptData` is still a string for backward compat (the template uses it as a string). No changes needed to `bootstrapScriptData` struct — the string representations are already built in `LoadConfig()`.

But the `ExtraCommands` field now gets populated from `cfg.BootstrapExtraCommands` which is built from `bootstrap.hooks` in the YAML. Verify this flows through correctly.

The template already has an `{{.ExtraCommands}}` placeholder. If it doesn't, add it:

In the bootstrap template (look for `main()`), add at the end of `main()`:

```bash
{{if .ExtraCommands}}
{{.ExtraCommands}}
{{end}}
```

- [ ] **Step 2: Update script write path**

In `bootstrap_script.go`, find where the generated script is written. Change from repo-local path to `filepath.Join(cfg.ConfigDir, "scripts", "guest-bootstrap.sh")`.

- [ ] **Step 3: Update `BootstrapSetup` to write YAML config instead of `.vmctl.env`**

In `vm.go`, `BootstrapSetup()` function writes preferences via `UpdateDotEnvFile()`. Replace with `SaveConfig()`:

```go
// In BootstrapSetup(), replace:
//   if err := UpdateDotEnvFile(DotEnvPath(cfg.RepoRoot), updates); err != nil {
// With:
//   newCfg.DefaultShell = shell
//   newCfg.DefaultEditor = editor
//   newCfg.WindowManager = wm
//   ...
//   if err := SaveConfig(newCfg); err != nil {
```

- [ ] **Step 4: Commit**

```bash
git add internal/vmctl/bootstrap_script.go internal/vmctl/vm.go
git commit -m "feat: update bootstrap for YAML config and hooks"
```

---

### Task 8: Update sync config to use in-memory Config

**Files:**
- Modify: `internal/vmctl/sync_config.go`
- Modify: `internal/vmctl/sync_cli.go`

- [ ] **Step 1: Add `yaml` struct tags to `SyncPair` and update `syncConfigPath`**

`SyncPair` fields need `yaml` tags (yaml.v3 does not automatically use `json` tags). Add them alongside existing `json` tags:

```go
type SyncPair struct {
	ID                  string        `json:"id" yaml:"id"`
	Mode                SyncMode      `json:"mode" yaml:"mode"`
	HostPath            string        `json:"host_path" yaml:"host_path"`
	VMPath              string        `json:"vm_path" yaml:"guest_path"`
	BareRepoPath        string        `json:"bare_repo_path,omitempty" yaml:"bare_repo_path,omitempty"`
	Direction           SyncDirection `json:"direction,omitempty" yaml:"direction,omitempty"`
	Exclude             []string      `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	ExcludeFrom         string        `json:"exclude_from,omitempty" yaml:"exclude_from,omitempty"`
	BackupRetentionDays int           `json:"backup_retention_days,omitempty" yaml:"backup_retention_days,omitempty"`
	CreatedAt           time.Time     `json:"created_at" yaml:"-"`
}
```

Note: `VMPath` → yaml tag `guest_path` to match the YAML schema; `CreatedAt` → `yaml:"-"` (auto-generated, not persisted).

Also update `syncConfigPath` to use ConfigDir:

```go
func syncConfigPath(cfg Config) string {
	return filepath.Join(cfg.ConfigDir, "vmctl.yaml")
}
```

- [ ] **Step 2: Rewrite `LoadSyncConfig` to read from Config.SyncPairs**

```go
func LoadSyncConfig(path string) (SyncConfig, error) {
	// This function is now called with the yaml path.
	// Instead of loading a separate JSON file, we load the full config.
	ycfg, err := loadVMConfigFile(path)
	if err != nil {
		return SyncConfig{}, err
	}
	return SyncConfig{Pairs: ycfg.Sync}, nil
}
```

- [ ] **Step 3: Rewrite `SaveSyncConfig` to write full YAML**

```go
func SaveSyncConfig(path string, scfg SyncConfig) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.SyncPairs = scfg.Pairs
	return SaveConfig(cfg)
}
```

- [ ] **Step 4: Update `sync_cli.go` calls**

Replace all `SaveSyncConfig(syncConfigPath(cfg), sc)` calls with the pattern above. Actually, since `SaveSyncConfig` now re-loads and writes the full YAML, the existing call sites work as-is. Verify they compile.

- [ ] **Step 5: Commit**

```bash
git add internal/vmctl/sync_config.go internal/vmctl/sync_cli.go
git commit -m "refactor: migrate sync config to in-memory YAML"
```

---

### Task 9: Update tunnel config to use in-memory Config

**Files:**
- Modify: `internal/vmctl/tunnel_config.go`
- Modify: `internal/vmctl/tunnel_cli.go`
- Modify: `internal/vmctl/tunnel_manager.go`

- [ ] **Step 1: Add `yaml` struct tags to `Tunnel` and update `tunnelConfigPath`**

Add `yaml` tags alongside existing `json` tags:

```go
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
```

Also update `tunnelConfigPath` to use ConfigDir:

```go
func tunnelConfigPath(cfg Config) string {
	return filepath.Join(cfg.ConfigDir, "vmctl.yaml")
}
```

- [ ] **Step 2: Rewrite `LoadTunnelConfig` to read from Config.Tunnels**

```go
func LoadTunnelConfig(path string) (TunnelConfig, error) {
	ycfg, err := loadVMConfigFile(path)
	if err != nil {
		return TunnelConfig{}, err
	}
	return TunnelConfig{Tunnels: ycfg.Tunnels}, nil
}
```

- [ ] **Step 3: Rewrite `SaveTunnelConfig` to write full YAML**

```go
func SaveTunnelConfig(path string, tcfg TunnelConfig) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Tunnels = tcfg.Tunnels
	return SaveConfig(cfg)
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/vmctl/tunnel_config.go internal/vmctl/tunnel_cli.go internal/vmctl/tunnel_manager.go
git commit -m "refactor: migrate tunnel config to in-memory YAML"
```

---

### Task 10: Update web handlers

**Files:**
- Modify: `internal/vmctl/web_handlers.go`

- [ ] **Step 1: Update `handleStatus` to return new config shape**

Update the config map to include new field names:

```go
"config": map[string]any{
	"shell":         cfg.DefaultShell,
	"editor":        cfg.DefaultEditor,
	"windowManager": cfg.WindowManager,
	"memoryMiB":     cfg.MemoryMiB,
	"diskSize":      cfg.DiskSize,
	"staticIP":      cfg.StaticIP,
	"brewPackages":  strings.Fields(cfg.BootstrapBrewPackages),
	"cargoPackages": parseCargoPackagesForWeb(cfg.BootstrapCargoPackages),
	"hooks":         strings.Split(strings.TrimSpace(cfg.BootstrapExtraCommands), "\n"),
},
```

Add helper:

```go
func parseCargoPackagesForWeb(raw string) []map[string]string {
	var result []map[string]string
	if raw == "" {
		return result
	}
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(entry, ":", 2)
		m := map[string]string{"crate": parts[0]}
		if len(parts) == 2 {
			m["command"] = parts[1]
		}
		result = append(result, m)
	}
	return result
}
```

- [ ] **Step 2: Update `handleBootstrap` to write YAML instead of `.vmctl.env`**

Replace the `UpdateDotEnvFile` call with `SaveConfig`:

```go
newCfg, err := LoadConfig()
if err != nil {
	return jsonError(c, http.StatusInternalServerError, err.Error())
}
newCfg.DefaultShell = req.Shell
newCfg.DefaultEditor = req.Editor
newCfg.WindowManager = req.WindowManager
// Convert brew/cargo from potentially new array format or string
if req.BrewPackages != "" {
	newCfg.BootstrapBrewPackages = req.BrewPackages
}
if req.CargoPackages != "" {
	newCfg.BootstrapCargoPackages = req.CargoPackages
}
if req.MemoryMiB > 0 {
	newCfg.MemoryMiB = req.MemoryMiB
}
if req.DiskSize != "" {
	newCfg.DiskSize = req.DiskSize
}
if req.StaticIP != "" {
	newCfg.StaticIP = req.StaticIP
}
if err := SaveConfig(newCfg); err != nil {
	return jsonError(c, http.StatusInternalServerError, err.Error())
}
```

- [ ] **Step 3: Update handleBootstrap request struct for new fields**

Add `Hooks string` to `bootstrapReq`.

- [ ] **Step 4: Commit**

```bash
git add internal/vmctl/web_handlers.go
git commit -m "refactor: update web handlers for YAML config"
```

---

### Task 11: Update web frontend

**Files:**
- Modify: `web/static/app.js`

- [ ] **Step 1: Update config parsing in frontend**

In `app.js`, find where brew/cargo packages are parsed from the API response. Update to handle both the old string format and new array format:

```javascript
// brewPackages: if array, join with space; if string, use as-is
const brew = Array.isArray(config.brewPackages)
    ? config.brewPackages.join(' ')
    : (config.brewPackages || '');

// cargoPackages: if array of objects, convert to comma-separated string
const cargo = Array.isArray(config.cargoPackages)
    ? config.cargoPackages.map(c => c.crate + ':' + (c.command || c.crate)).join(',')
    : (config.cargoPackages || '');

// hooks: new field
const hooks = Array.isArray(config.hooks)
    ? config.hooks.join('\n')
    : (config.hooks || '');
```

- [ ] **Step 2: Add hooks UI in the bootstrap modal**

Add a textarea for post-bootstrap hooks in the bootstrap configuration modal. Label: "Post-bootstrap hooks (one command per line)".

- [ ] **Step 3: Commit**

```bash
git add web/static/app.js
git commit -m "feat: update web UI for YAML config and hooks"
```

---

### Task 12: Write YAML config tests

**Files:**
- Create: `internal/vmctl/yaml_config_test.go`

- [ ] **Step 1: Write failing tests**

```go
package vmctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVMConfigFileDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vmctl.yaml")

	// Empty file
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
  brew_packages:
    - helix
    - zig
  cargo_packages:
    - crate: fresh-editor
      command: fresh
  hooks:
    - "echo done"

git:
  user_name: Test User
  user_email: test@example.com

sync:
  - name: proj
    host_path: /host/proj
    guest_path: /guest/proj
    mode: copy
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
	if len(cfg.Bootstrap.BrewPackages) != 2 {
		t.Errorf("expected 2 brew packages, got %d", len(cfg.Bootstrap.BrewPackages))
	}
	if len(cfg.Bootstrap.CargoPackages) != 1 {
		t.Errorf("expected 1 cargo package, got %d", len(cfg.Bootstrap.CargoPackages))
	}
	if len(cfg.Bootstrap.Hooks) != 1 {
		t.Errorf("expected 1 hook, got %d", len(cfg.Bootstrap.Hooks))
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
	cfg.Bootstrap.BrewPackages = []string{"test-pkg"}

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
	if len(reloaded.Bootstrap.BrewPackages) != 1 {
		t.Errorf("expected 1 brew package, got %d", len(reloaded.Bootstrap.BrewPackages))
	}
}
```

- [ ] **Step 2: Run tests and verify they pass**

```bash
go test ./internal/vmctl/... -run TestLoadVMConfig -v
go test ./internal/vmctl/... -run TestSaveAndReloadConfig -v
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add internal/vmctl/yaml_config_test.go
git commit -m "test: add YAML config parsing tests"
```

---

### Task 13: Update existing config tests

**Files:**
- Modify: `internal/vmctl/config_test.go`

- [ ] **Step 1: Rewrite tests for YAML-based config**

Read the current `config_test.go`. Replace env-var-based tests with tests that:
1. Create a temp `vmctl.yaml` with `VMCTL_CONFIG_DIR` set to the temp dir
2. Call `LoadConfig()` 
3. Verify the `Config` struct has correct values

Example test:

```go
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
}
```

Add tests for: SSH key resolution, brew/cargo package stringification, hooks population, missing file handling.

- [ ] **Step 2: Run tests**

```bash
go test ./internal/vmctl/... -run TestLoadConfig -v
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add internal/vmctl/config_test.go
git commit -m "test: update config tests for YAML-based loading"
```

---

### Task 14: Update sync/tunnel tests

**Files:**
- Modify: `internal/vmctl/sync_config_test.go`
- Modify: `internal/vmctl/tunnel_config_test.go`
- Modify: `internal/vmctl/tunnel_manager_test.go`

- [ ] **Step 1: Update `sync_config_test.go`**

Replace any test that writes `.vmctl.sync` JSON with tests that set `Config.SyncPairs` directly and call `SaveConfig()`. The load/save functions should now work through the full YAML file.

Adapt test setup: each test creates a temp `VMCTL_CONFIG_DIR`, writes a minimal `vmctl.yaml`, loads config, modifies sync pairs, saves, reloads, verifies.

- [ ] **Step 2: Update `tunnel_config_test.go` and `tunnel_manager_test.go`**

Same pattern as sync — replace `.vmctl.tunnels` JSON file tests with YAML-based config tests.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/vmctl/... -run "TestSync|TestTunnel" -v
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/vmctl/sync_config_test.go internal/vmctl/tunnel_config_test.go internal/vmctl/tunnel_manager_test.go
git commit -m "test: update sync/tunnel tests for YAML config"
```

---

### Task 15: Update E2E test

**Files:**
- Modify: `scripts/e2e-test.sh`

- [ ] **Step 1: Update e2e-test.sh for new paths**

Replace references to repo-local `.vmctl.env` with `VMCTL_CONFIG_DIR` pointing to the test state dir. Replace `.vm/void-dev/` paths with the new config dir structure.

```bash
E2E_CONFIG_DIR="${TEST_ROOT}/config"
mkdir -p "${E2E_CONFIG_DIR}/scripts"
mkdir -p "${E2E_CONFIG_DIR}/images"
mkdir -p "${E2E_CONFIG_DIR}/void-dev"

export VMCTL_CONFIG_DIR="${E2E_CONFIG_DIR}"

# Write minimal vmctl.yaml for e2e
cat > "${E2E_CONFIG_DIR}/vmctl.yaml" <<'YAML'
vm:
  cpus: 2
  memory_mib: 2048
  disk_size: "20G"
user:
  name: vm
  password: vm
guest:
  timezone: UTC
YAML
```

Update `vmctl()` function to no longer `cd` to repo root (or keep it for binary location, but ensure config dir is set).

- [ ] **Step 2: Commit**

```bash
git add scripts/e2e-test.sh
git commit -m "test: update e2e test for YAML config paths"
```

---

### Task 16: Update cobra.go Usage text

**Files:**
- Modify: `internal/vmctl/cobra.go`
- Modify: `internal/vmctl/config.go` (Usage function)

- [ ] **Step 1: Update `Usage()` text in config.go**

Remove all env var references from the Usage text. Replace with:
```
Configuration: ~/.config/agent-vm/vmctl.yaml
Override with: VMCTL_CONFIG_DIR=/custom/path
```

- [ ] **Step 2: Commit**

```bash
git add internal/vmctl/config.go internal/vmctl/cobra.go
git commit -m "docs: update usage text for YAML config"
```

---

### Task 17: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Update paths and conventions**

Replace `.vmctl.env` mentions with `~/.config/agent-vm/vmctl.yaml`. Replace `.vm/void-dev/` with `~/.config/agent-vm/void-dev/`. Update default user to `vm`. Remove `.vmctl.*` gitignore references.

- [ ] **Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: update AGENTS.md for YAML config migration"
```

---

### Task 18: Update .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Remove `.vmctl.*` entries**

Remove from `.gitignore`:
```
.vmctl.env
.vmctl.*
```

Since config now lives in `~/.config/agent-vm/`, these are no longer repo-local files.

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: remove .vmctl.* from gitignore"
```

---

### Task 19: Remove dotenv.go

**Files:**
- Delete: `internal/vmctl/dotenv.go`

- [ ] **Step 1: Remove dotenv.go**

The `.vmctl.env` KEY=VALUE format is no longer used. Remove `dotenv.go` and its tests (`dotenv_test.go` if it exists).

- [ ] **Step 2: Verify build compiles without dotenv.go**

```bash
go build ./internal/vmctl/...
go test ./internal/vmctl/...
```

- [ ] **Step 3: Commit**

```bash
git rm internal/vmctl/dotenv.go
git commit -m "refactor: remove dotenv.go (replaced by YAML config)"
```

---

### Task 20: Final integration test

**Files:**
- None (verification only)

- [ ] **Step 1: Run all unit tests**

```bash
go test -short ./internal/vmctl/... -v
```

Expected: all tests pass.

- [ ] **Step 2: Run build verification**

```bash
go build ./cmd/vmctl
```

Expected: clean build.

- [ ] **Step 3: Verify e2e test works**

```bash
./scripts/e2e-test.sh
```

Expected: VM boots, SSH works, bootstrap completes, chromium reachable.

- [ ] **Step 4: Commit**

```bash
git commit -m "verify: all tests pass after YAML config migration" --allow-empty
```

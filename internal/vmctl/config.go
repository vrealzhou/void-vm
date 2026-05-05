package vmctl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	RepoRoot               string
	Name                   string
	StateDir               string
	DiskPath               string
	KernelPath             string
	InitrdPath             string
	BootstrapMarker        string
	EFIVarsPath            string
	PIDFile                string
	RestSocket             string
	LogFile                string
	SerialLog              string
	CPUs                   int
	MemoryMiB              int
	DiskSize               string
	MAC                    string
	StaticIP               string
	Gateway                string
	CIDR                   int
	DNSServers             string
	SSHUser                string
	GuestUser              string
	GuestPassword          string
	RootPassword           string
	SSHPublicKey           string
	SSHPrivateKey          string
	SSHKnownHostsFile      string
	Timezone               string
	DefaultShell           string
	DefaultEditor          string
	WindowManager          string
	StarshipPresetURL      string
	BootstrapBrewPackages  string
	BootstrapCargoPackages string
	GitUserName            string
	GitUserEmail           string
	SetDefaultShell        bool
	BootstrapExtraCommands string
	VoidRepository         string
	ImageDir               string
	BaseImage              string
	BaseImageURL           string
	BuildKernelURL         string
	GUI                    bool
	Width                  int
	Height                 int
	ConfigDir  string
	SyncPairs  []SyncPair
	Tunnels    []Tunnel
}

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

	sshPublicKey := vcfg.User.SSHPublicKey
	if sshPublicKey == "" {
		sshPublicKey = filepath.Join(homeDir, ".ssh", "id_ed25519.pub")
	}

	dnsServersStr := strings.Join(vcfg.Network.DNSServers, " ")

	brewPackagesStr := strings.Join(vcfg.Bootstrap.BrewPackages, " ")

	var cargoParts []string
	for _, cp := range vcfg.Bootstrap.CargoPackages {
		cmd := cp.Command
		if cmd == "" {
			cmd = cp.Crate
		}
		cargoParts = append(cargoParts, cp.Crate+":"+cmd)
	}
	cargoPackagesStr := strings.Join(cargoParts, ",")

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
		GUI:                    *vcfg.VM.GUI,
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

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Usage(cfg Config) string {
	return fmt.Sprintf(`Usage:
  go run ./cmd/vmctl           # open the web UI
  go run ./cmd/vmctl <command>

Commands:
  start      Create missing assets and start the VM
  stop       Stop the VM via vfkit REST API
  destroy    Stop the VM and remove generated VM state and disk files
  status     Show VM state and effective network target
  gui        Open the web VM control panel
  bootstrap  Run the guided bootstrap flow and write bootstrap.done
  clip-in    Copy the macOS clipboard into the guest Wayland clipboard
  clip-out   Copy the guest Wayland clipboard into the macOS clipboard
  ssh        SSH into the guest using the configured static IP
  ip         Print the configured guest IP
  sync       Manage file sync pairs between host and VM
  tunnel     Manage SSH tunnels

Configuration: %s/vmctl.yaml
Override with: VMCTL_CONFIG_DIR=/custom/path

Defaults:
  VM:          6 CPU / 6144 MiB RAM / 100 GiB disk / 1920x1200
  Network:     192.168.64.10 / gateway 192.168.64.1
  User:        vm / password dev, root password root
  Shell:       fish, editor: neovim, WM: sway

`, cfg.ConfigDir)
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

func SaveConfig(cfg Config) error {
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		return err
	}
	yamlPath := filepath.Join(cfg.ConfigDir, "vmctl.yaml")

	vcfg := VMConfigFile{}
	vcfg.VM.Name = cfg.Name
	vcfg.VM.CPUs = cfg.CPUs
	vcfg.VM.MemoryMiB = cfg.MemoryMiB
	vcfg.VM.DiskSize = cfg.DiskSize
	vcfg.VM.GUI = &cfg.GUI
	vcfg.VM.Width = cfg.Width
	vcfg.VM.Height = cfg.Height
	vcfg.Network.StaticIP = cfg.StaticIP
	vcfg.Network.Gateway = cfg.Gateway
	vcfg.Network.CIDR = cfg.CIDR
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

	if cfg.DNSServers != "" {
		vcfg.Network.DNSServers = strings.Fields(cfg.DNSServers)
	}

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

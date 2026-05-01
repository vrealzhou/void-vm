package vmctl

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
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
	VoidRepository         string
	ImageDir               string
	BaseImage              string
	BaseImageURL           string
	GUI                    bool
	Width                  int
	Height                 int
}

func LoadConfig() (Config, error) {
	repoRoot, err := determineRepoRoot()
	if err != nil {
		return Config{}, err
	}

	env := map[string]string{}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	dotEnvPath := filepath.Join(repoRoot, ".vmctl.env")
	if _, err := os.Stat(dotEnvPath); err == nil {
		fileEnv, err := parseDotEnv(dotEnvPath)
		if err != nil {
			return Config{}, err
		}
		maps.Copy(env, fileEnv)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{RepoRoot: repoRoot}
	cfg.Name = envOr(env, "VM_NAME", "void-dev")
	cfg.StateDir = envOr(env, "VM_STATE_DIR", filepath.Join(repoRoot, ".vm", cfg.Name))
	cfg.DiskPath = envOr(env, "VM_DISK_PATH", filepath.Join(cfg.StateDir, "disk.img"))
	cfg.KernelPath = envOr(env, "VM_KERNEL_PATH", filepath.Join(cfg.StateDir, "vmlinuz"))
	cfg.InitrdPath = envOr(env, "VM_INITRD_PATH", filepath.Join(cfg.StateDir, "initramfs.img"))
	cfg.BootstrapMarker = envOr(env, "VM_BOOTSTRAP_MARKER", filepath.Join(cfg.StateDir, "bootstrap.done"))
	cfg.EFIVarsPath = envOr(env, "VM_EFI_VARS_PATH", filepath.Join(cfg.StateDir, "efi-vars.fd"))
	cfg.PIDFile = envOr(env, "VM_PID_FILE", filepath.Join(cfg.StateDir, "vfkit.pid"))
	cfg.RestSocket = envOr(env, "VM_REST_SOCKET", filepath.Join(cfg.StateDir, "vfkit.sock"))
	cfg.LogFile = envOr(env, "VM_LOG_FILE", filepath.Join(cfg.StateDir, "vfkit.log"))
	cfg.SerialLog = envOr(env, "VM_SERIAL_LOG", filepath.Join(cfg.StateDir, "serial.log"))

	if cfg.CPUs, err = intEnv(env, "VM_CPUS", 6); err != nil {
		return Config{}, err
	}
	if cfg.MemoryMiB, err = intEnv(env, "VM_MEMORY_MIB", 6144); err != nil {
		return Config{}, err
	}
	cfg.DiskSize = envOr(env, "VM_DISK_SIZE", "100G")
	cfg.MAC = envOr(env, "VM_MAC", "52:54:00:64:00:10")
	cfg.StaticIP = envOr(env, "VM_STATIC_IP", "192.168.64.10")
	cfg.Gateway = envOr(env, "VM_GATEWAY", "192.168.64.1")
	if cfg.CIDR, err = intEnv(env, "VM_CIDR", 24); err != nil {
		return Config{}, err
	}
	cfg.DNSServers = envOr(env, "VM_DNS_SERVERS", "1.1.1.1,8.8.8.8")

	cfg.SSHUser = envOr(env, "VM_SSH_USER", "dev")
	cfg.GuestUser = envOr(env, "VM_GUEST_USER", "dev")
	cfg.GuestPassword = envOr(env, "VM_GUEST_PASSWORD", "dev")
	cfg.RootPassword = envOr(env, "VM_ROOT_PASSWORD", "root")
	cfg.SSHPublicKey = envOr(env, "VM_SSH_PUBLIC_KEY", filepath.Join(homeDir, ".ssh", "id_ed25519.pub"))
	cfg.SSHKnownHostsFile = envOr(env, "VM_SSH_KNOWN_HOSTS_FILE", "")
	cfg.Timezone = envOr(env, "VM_TIMEZONE", "Australia/Sydney")
	if cfg.DefaultShell, err = choiceEnv(env, "VM_DEFAULT_SHELL", "fish", "fish", "zsh"); err != nil {
		return Config{}, err
	}
	if cfg.DefaultEditor, err = choiceEnv(env, "VM_DEFAULT_EDITOR", "neovim", "neovim", "helix"); err != nil {
		return Config{}, err
	}
	if cfg.WindowManager, err = choiceEnv(env, "VM_WINDOW_MANAGER", "sway", "sway", "xfce"); err != nil {
		return Config{}, err
	}
	cfg.StarshipPresetURL = envOr(env, "VM_STARSHIP_PRESET_URL", "https://starship.rs/presets/toml/tokyo-night.toml")
	cfg.BootstrapBrewPackages = envOr(env, "VM_BOOTSTRAP_BREW_PACKAGES", "")
	cfg.BootstrapCargoPackages = envOr(env, "VM_BOOTSTRAP_CARGO_PACKAGES", "")
	cfg.GitUserName = envOr(env, "VM_GIT_USER_NAME", "")
	cfg.GitUserEmail = envOr(env, "VM_GIT_USER_EMAIL", "")
	if cfg.SetDefaultShell, err = boolEnv(env, "VM_SET_DEFAULT_SHELL", true); err != nil {
		return Config{}, err
	}
	cfg.VoidRepository = envOr(env, "VM_VOID_REPOSITORY", "https://repo-default.voidlinux.org")

	cfg.ImageDir = envOr(env, "VM_IMAGE_DIR", filepath.Join(repoRoot, "images"))
	cfg.BaseImage = envOr(env, "VM_BASE_IMAGE", filepath.Join(cfg.ImageDir, "void-aarch64-ROOTFS-20250202.tar.xz"))
	cfg.BaseImageURL = envOr(env, "VM_BASE_IMAGE_URL", "https://repo-default.voidlinux.org/live/current/void-aarch64-ROOTFS-20250202.tar.xz")
	if cfg.GUI, err = boolEnv(env, "VM_GUI", true); err != nil {
		return Config{}, err
	}
	if cfg.Width, err = intEnv(env, "VM_WIDTH", 1920); err != nil {
		return Config{}, err
	}
	if cfg.Height, err = intEnv(env, "VM_HEIGHT", 1200); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Usage(cfg Config) string {
	return fmt.Sprintf(`Usage:
  go run ./cmd/vmctl           # open the GUI control panel
  go run ./cmd/vmctl <command>

Commands:
  start      Create missing assets and start the VM
  stop       Stop the VM via vfkit REST API
  destroy    Stop the VM and remove generated VM state and disk files
  status     Show VM state and effective network target
  gui        Open the Fyne VM control panel
  bootstrap  Run the guided bootstrap flow and write bootstrap.done
  clip-in    Copy the macOS clipboard into the guest Wayland clipboard
  clip-out   Copy the guest Wayland clipboard into the macOS clipboard
  ssh        SSH into the guest using the configured static IP
  ip         Print the configured guest IP
  sync       Manage file sync pairs between host and VM
  tunnel     Manage SSH tunnels

Important environment variables:
  VM_MEMORY_MIB=6144
  VM_DISK_SIZE=100G
  VM_STATIC_IP=192.168.64.10
  VM_GATEWAY=192.168.64.1
  VM_IMAGE_DIR=%s
  VM_BASE_IMAGE=%s
  VM_BASE_IMAGE_URL=%s
  VM_SSH_USER=dev
  VM_GUEST_USER=dev
  VM_GUEST_PASSWORD=dev
  VM_ROOT_PASSWORD=root
  VM_SSH_PUBLIC_KEY=%s
  VM_TIMEZONE=%s
  VM_DEFAULT_SHELL=fish
  VM_DEFAULT_EDITOR=neovim
  VM_WINDOW_MANAGER=sway
  VM_VOID_REPOSITORY=%s
  VM_STARSHIP_PRESET_URL=https://starship.rs/presets/toml/tokyo-night.toml
  VM_BOOTSTRAP_BREW_PACKAGES="helix zellij zig opencode lazygit gitui"
  VM_BOOTSTRAP_CARGO_PACKAGES="fresh-editor:fresh"
  VM_GIT_USER_NAME="Your Name"
  VM_GIT_USER_EMAIL="you@example.com"
  VM_GUI=1

Notes:
  - Running vmctl with no subcommand opens the GUI control panel.
  - By default vmctl uses the official Void Linux aarch64 glibc ROOTFS tarball
    and builds a bootable raw disk with podman.
  - The generated VM uses direct kernel boot via vfkit --kernel/--initrd and
    stores boot assets at %s and %s.
  - On first successful boot, vmctl waits for SSH and runs bootstrap
    automatically once, then records %s.
  - If %s does not exist, vmctl downloads it automatically to
    %s.
`, cfg.ImageDir, cfg.BaseImage, cfg.BaseImageURL, cfg.SSHPublicKey, cfg.Timezone, cfg.VoidRepository, cfg.KernelPath, cfg.InitrdPath, cfg.BootstrapMarker, cfg.BaseImage, cfg.ImageDir)
}

func determineRepoRoot() (string, error) {
	if root := os.Getenv("VMCTL_REPO_ROOT"); root != "" {
		return filepath.Clean(root), nil
	}
	return os.Getwd()
}

func parseDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNumber)
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func envOr(env map[string]string, key, fallback string) string {
	if value, ok := env[key]; ok && value != "" {
		return value
	}
	return fallback
}

func intEnv(env map[string]string, key string, fallback int) (int, error) {
	if value, ok := env[key]; ok && value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return n, nil
	}
	return fallback, nil
}

func boolEnv(env map[string]string, key string, fallback bool) (bool, error) {
	if value, ok := env[key]; ok && value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return true, nil
		case "0", "false", "no", "off":
			return false, nil
		default:
			return false, fmt.Errorf("%s must be a boolean", key)
		}
	}
	return fallback, nil
}

func choiceEnv(env map[string]string, key, fallback string, allowed ...string) (string, error) {
	value := fallback
	if raw, ok := env[key]; ok && raw != "" {
		value = raw
	}
	for _, option := range allowed {
		if value == option {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s must be one of: %s", key, strings.Join(allowed, ", "))
}

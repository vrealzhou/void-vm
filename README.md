# void-vm

Use `vfkit` on Apple Silicon macOS to run a fixed-IP `arm64` Linux development VM.

The project is intentionally scoped to one supported workflow:

- Distribution: `Void Linux aarch64 glibc`
- Desktop: configurable, default `Sway`
- Entry point: `go run ./cmd/vmctl <command>`
- Network model: fixed IP `192.168.64.10`
- Default user: `dev`
- Default resources: `6 CPU / 6 GiB RAM / 100 GiB disk`

This is not a general-purpose VM abstraction layer. The goal is narrower: build a fast, reproducible, scriptable personal dev VM that supports both SSH and GUI use.

## Supported Scope

The project supports exactly two kinds of input images:

1. The official Void `ROOTFS tarball`
2. An existing disk image: `.img`, `.img.xz`, `.raw`, `.raw.xz`, or `.qcow2`

These older paths are no longer maintained:

- installer ISO
- cloud-init
- interactive installer workflows

That is deliberate. They add code and maintenance cost without improving the current default workflow.

## Host Requirements

The host machine needs these commands:

```bash
vfkit
qemu-img
curl
ssh
go
podman
pbcopy
pbpaste
```

Quick check:

```bash
command -v vfkit qemu-img curl ssh go podman pbcopy pbpaste
```

## Install And First Boot

There is no separate installer step for the repo itself. The default entry point is the GUI:

```bash
go run ./cmd/vmctl
```

From there you can bootstrap, start, stop, and destroy the VM.

To boot directly from the CLI, run:

```bash
go run ./cmd/vmctl start
```

On the first successful `start`, `vmctl` will:

1. Download the official Void rootfs into the repo-root `images/` directory by default
2. Build a raw disk
3. Write users, passwords, SSH keys, fixed networking, GUI, and system configuration offline
4. Extract `vmlinuz` and `initramfs`
5. Start the VM
6. Wait for SSH and run bootstrap once

If startup succeeds:

- the GUI auto-enters the configured desktop session
- the host can connect with:

```bash
go run ./cmd/vmctl ssh
```

To inspect current state:

```bash
go run ./cmd/vmctl status
```

To print only the guest IP:

```bash
go run ./cmd/vmctl ip
```

## Default Layout

Default directories:

```text
images/

.vm/
  void-dev/
    disk.img
    vmlinuz
    initramfs.img
    bootstrap.done
    efi-vars.fd
    vfkit.log
    serial.log
    vfkit.pid
```

Default resources:

```text
CPU:    6
Memory: 6144 MiB
Disk:   100G
IP:     192.168.64.10
```

Default login:

- SSH user: `dev`
- Local password: `dev / dev`
- Root password: `root / root`
- SSH public key: host `~/.ssh/id_ed25519.pub`

## Daily Commands

```bash
go run ./cmd/vmctl start
go run ./cmd/vmctl stop
go run ./cmd/vmctl destroy
go run ./cmd/vmctl status
go run ./cmd/vmctl ssh
go run ./cmd/vmctl ip
go run ./cmd/vmctl gui
go run ./cmd/vmctl bootstrap
go run ./cmd/vmctl clip-in
go run ./cmd/vmctl clip-out
```

Meaning:

- `start`: create any missing assets and boot the VM
- `stop`: stop the VM
- `destroy`: stop the VM and remove generated state and disk files, but keep the base image
- `status`: print current state, PID, disk path, and IP
- `ssh`: log into the guest using the configured default user
- `ip`: print only the guest IP
- `gui`: open the Fyne control panel
- `bootstrap`: open the guided bootstrap flow, apply the chosen preferences, and write `bootstrap.done`
- `clip-in`: copy macOS clipboard into the guest Wayland clipboard
- `clip-out`: copy the guest Wayland clipboard back into macOS

The GUI control panel can:

- guide the user to bootstrap first through a popup that asks for shell, editor, and window manager
- keep `start` and `stop` disabled until bootstrap has completed
- start, stop, and destroy the VM after bootstrap
- show whether the VM is running
- sample guest CPU and memory usage over SSH

## Guest Bootstrap Contents

Bootstrap configures:

- `fish` or `zsh`
- `starship` with the Tokyo Night preset
- `fnm` for Node.js
- `Rust` and `cargo`
- `Homebrew for Linux`
- `Neovim`
- `Helix`, `Zellij`, and `Zig`
- `opencode`, `lazygit`, and `gitui`
- `Ghostty`
- `Chromium`
- `Zen Browser`
- `Fcitx5` Chinese input
- `~/.gitconfig`
- `tty1 autologin -> dev -> configured desktop session`

Default desktop behavior:

- `Super + Enter`: open `ghostty`
- `Super + D`: open `wofi --show drun`
- Pointer scrolling: natural scrolling, aligned with macOS
- IME switch: left `Shift`
- Fallback IME switch: `Caps Lock`
- `swaybar` shows current IME state

Browser notes:

- `Chromium` is wrapped to use `Xwayland`
- `Zen Browser` uses native `Wayland`

## Customization

Put overrides in a repo-root `.vmctl.env`. Any value you omit keeps the code default.
Use absolute paths in `.vmctl.env` when you override file locations. The loader does not expand `~` or `$HOME`.

You can also open the GUI directly:

```bash
go run ./cmd/vmctl gui
```

Running `go run ./cmd/vmctl` without a subcommand opens the same control panel. Preferences are chosen from the `Bootstrap` popup, not from the main screen.

You can start from the template:

```bash
cp .vmctl.env.example .vmctl.env
```

Template file:
[.vmctl.env.example](/Users/zhouye/vms/vfkit-void/.vmctl.env.example)

### Resources

```bash
VM_CPUS=6
VM_MEMORY_MIB=6144
VM_DISK_SIZE=100G
```

### Networking

```bash
VM_STATIC_IP=192.168.64.10
VM_GATEWAY=192.168.64.1
VM_CIDR=24
VM_DNS_SERVERS=1.1.1.1,8.8.8.8
VM_MAC=52:54:00:64:00:10
```

### Accounts And SSH

```bash
VM_SSH_USER=dev
VM_GUEST_USER=dev
VM_GUEST_PASSWORD=dev
VM_ROOT_PASSWORD=root
VM_SSH_PUBLIC_KEY=/absolute/path/to/id_ed25519.pub
VM_SSH_KNOWN_HOSTS_FILE=/absolute/path/to/known_hosts
```

If `VM_SSH_KNOWN_HOSTS_FILE` is not set, `vmctl ssh` uses:

```text
StrictHostKeyChecking=no
UserKnownHostsFile=/dev/null
```

### Image Source

Default values:

```bash
VM_IMAGE_DIR=/absolute/path/to/void-vm/images
VM_BASE_IMAGE=/absolute/path/to/void-vm/images/void-aarch64-ROOTFS-20250202.tar.xz
VM_BASE_IMAGE_URL=https://repo-default.voidlinux.org/live/current/void-aarch64-ROOTFS-20250202.tar.xz
```

By default, `vmctl` resolves `VM_IMAGE_DIR` to the repo-root `images/` directory. If you already have a disk image elsewhere, you can just set:

```bash
VM_BASE_IMAGE=/absolute/path/to/custom.img
```

### Timezone And Display

```bash
VM_TIMEZONE=Australia/Sydney
VM_DEFAULT_SHELL=fish
VM_DEFAULT_EDITOR=neovim
VM_WINDOW_MANAGER=sway
VM_STARSHIP_PRESET_URL=https://starship.rs/presets/toml/tokyo-night.toml
VM_GUI=1
VM_WIDTH=1920
VM_HEIGHT=1200
```

Bootstrap writes the prompt config to `~/.config/starship.toml` and enables it from Fish with `starship init fish | source`. The Tokyo Night preset uses Nerd Font symbols, so make sure your terminal font supports them.
Bootstrap installs `fnm` through Homebrew, enables it in the configured shell, installs the latest LTS Node.js release, and uses that instead of a Homebrew-managed `node`.

For 4K:

```bash
VM_WIDTH=3840
VM_HEIGHT=2160
```

Resolution changes require a VM restart.

## Custom Bootstrap Package Lists

### Homebrew Packages

```bash
VM_BOOTSTRAP_BREW_PACKAGES="helix zellij zig opencode lazygit gitui"
```

Rules:

- split formula names with spaces
- bootstrap installs only what you list

### Cargo Packages

```bash
VM_BOOTSTRAP_CARGO_PACKAGES="fresh-editor:fresh,bacon:bacon,watchexec-cli:watchexec"
```

Rules:

- split entries with commas
- each entry uses `crate:command`
- `crate` is the name passed to `cargo install`
- `command` is the executable checked before install
- if the command already exists, bootstrap skips that package

If you want to hard-code the default package list in the script, edit:
[scripts/guest-bootstrap.sh](/Users/zhouye/vms/vfkit-void/scripts/guest-bootstrap.sh)

## Git Configuration

Bootstrap initializes `~/.gitconfig` with at least:

- `core.editor = nvim`
- `init.defaultBranch = main`
- `push.autoSetupRemote = true`
- `rebase.autoStash = true`
- `merge.conflictstyle = zdiff3`

To also write Git identity:

```bash
VM_GIT_USER_NAME="Your Name"
VM_GIT_USER_EMAIL="you@example.com"
```

## Clipboard

Host and guest clipboard sharing is not system-level seamless sync. It is implemented as helper commands:

```bash
go run ./cmd/vmctl clip-in
go run ./cmd/vmctl clip-out
```

Requirements:

- the guest is already in `Sway`
- the current Wayland session is available

## Shared Zellij Sessions

GUI and interactive SSH sessions share the same `XDG_RUNTIME_DIR` by default:

```text
/home/dev/.local/run
```

That means zellij sessions created in the GUI are visible over SSH as well.

## Rebuild And Troubleshooting

To rerun only guest-side initialization:

```bash
go run ./cmd/vmctl bootstrap
```

To rebuild the VM from scratch:

```bash
rm -rf .vm/void-dev
go run ./cmd/vmctl start
```

Useful state files:

- Log: [.vm/void-dev/vfkit.log](/Users/zhouye/vms/vfkit-void/.vm/void-dev/vfkit.log)
- Serial log: [.vm/void-dev/serial.log](/Users/zhouye/vms/vfkit-void/.vm/void-dev/serial.log)

## E2E

End-to-end test script:
[scripts/e2e-test.sh](/Users/zhouye/vms/vfkit-void/scripts/e2e-test.sh)

Run:

```bash
./scripts/e2e-test.sh
```

The script verifies:

- boot
- SSH
- bootstrap marker
- required commands
- headless Chromium reachability
- no unexpected second bootstrap after restart

## Code Layout

```text
cmd/vmctl/main.go           CLI entry point
internal/vmctl/cobra.go    Cobra command definitions
internal/vmctl/config.go   config loading and help text
internal/vmctl/util.go     shared helpers
internal/vmctl/vm.go       VM lifecycle and disk-building logic
scripts/guest-bootstrap.sh guest-side initialization
scripts/e2e-test.sh        end-to-end test
```

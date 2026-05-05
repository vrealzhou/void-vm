# Agent VM

Use `vfkit` on Apple Silicon macOS to run a fixed-IP `arm64` Linux development VM.

The project is intentionally scoped to one supported workflow:

- Distribution: `Void Linux aarch64 glibc`
- Desktop: configurable, default `Sway`
- Entry point: `go run ./cmd/agent-vm <command>`
- Network model: fixed IP `192.168.64.10`
- Default user: `vm`
- Default resources: `6 CPU / 6 GiB RAM / 100 GiB disk`

This is not a general-purpose VM abstraction layer. The goal is narrower: build a fast, reproducible, scriptable personal dev VM that supports both SSH and GUI use.

## Host Requirements

```bash
vfkit
qemu-img
curl
ssh
go
```

Optional (fallback disk builder):
```bash
podman
```

Quick check:

```bash
command -v vfkit qemu-img curl ssh go
```

## Quick Start

```bash
go run ./cmd/agent-vm start
```

On first boot, `agent-vm` downloads the Void rootfs tarball, builds a disk image, extracts the kernel/initramfs, boots the VM, and runs bootstrap automatically. Subsequent starts reuse the existing VM state.

### Web UI

```bash
go run ./cmd/agent-vm          # open web UI at http://localhost:8080
```

From the UI you can configure bootstrap preferences (shell, editor, window manager, brew/cargo packages, post-bootstrap hooks), start/stop/destroy the VM, manage sync pairs and SSH tunnels.

## Configuration

All config lives in `~/.config/agent-vm/vmctl.yaml`. Override the config directory with `VMCTL_CONFIG_DIR`.

```yaml
# example vmctl.yaml
vm:
  name: void-dev
  cpus: 6
  memory_mib: 6144
  disk_size: "100G"
  gui: true
  width: 1920
  height: 1200

network:
  static_ip: "192.168.64.10"
  gateway: "192.168.64.1"
  cidr: 24
  dns_servers: ["1.1.1.1", "8.8.8.8"]
  mac: "52:54:00:64:00:10"

user:
  name: vm
  password: dev
  root_password: root
  ssh_public_key: ""          # blank = auto-detect ~/.ssh/id_ed25519.pub

guest:
  timezone: Australia/Sydney
  default_shell: fish
  default_editor: neovim
  window_manager: sway

bootstrap:
  brew_packages:
    - helix
    - zellij
    - zig
    - opencode
    - lazygit
    - gitui
  cargo_packages:
    - crate: fresh-editor
      command: fresh
  hooks:
    - "echo bootstrap complete"

git:
  user_name: ""
  user_email: ""

sync:
  - name: myproject
    host_path: /Users/me/projects/myproject
    target_path: /home/vm/myproject
    mode: copy
    direction: host-to-vm

tunnels:
  - name: webapp
    type: local
    local_port: 3000
    remote_port: 3000
```

Every key is optional — defaults apply for anything omitted.

## Default Layout

```
~/.config/agent-vm/
├── vmctl.yaml
├── scripts/
│   └── guest-bootstrap.sh   # generated at bootstrap time
├── images/                  # base Void rootfs tarballs
└── void-dev/                # runtime state
    ├── disk.img
    ├── vmlinuz
    ├── initramfs.img
    ├── bootstrap.done
    ├── vfkit.log
    ├── serial.log
    └── vfkit.pid
```

Default login:

- User `vm` / password `dev`
- Root password `root`
- SSH key auto-detected from `~/.ssh/id_ed25519.pub`

## CLI Commands

```bash
go run ./cmd/agent-vm start       # create assets + boot VM
go run ./cmd/agent-vm stop        # stop the VM
go run ./cmd/agent-vm destroy     # stop + remove VM state
go run ./cmd/agent-vm status      # VM state, PID, IP, disk path
go run ./cmd/agent-vm ssh         # SSH into guest as user "vm"
go run ./cmd/agent-vm ip          # print guest IP
go run ./cmd/agent-vm bootstrap   # run bootstrap flow
go run ./cmd/agent-vm sync        # manage sync pairs
go run ./cmd/agent-vm tunnel      # manage SSH tunnels
```

## Sync

File sync between host and VM — supports two modes:

**copy** — rsync with backups:
```bash
go run ./cmd/agent-vm sync add --host-path /Users/me/projects/foo --target-path /home/vm/foo --mode copy
```

**git** — bare repo on VM, host pushes/pulls via `git push vm` / `git pull vm`:
```bash
go run ./cmd/agent-vm sync add --host-path /Users/me/projects/foo --target-path /home/vm/foo --mode git
```

Sync pairs can also be configured in `vmctl.yaml` or via the web UI.

## Tunnels

SSH port forwarding between host and VM:

```bash
go run ./cmd/agent-vm tunnel add --name webapp --type local --local-port 3000 --remote-port 3000
```

Also configurable in `vmctl.yaml` under the `tunnels:` section.

## Guest Bootstrap

Bootstrap configures inside the VM:

- `fish` or `zsh` shell
- `starship` prompt
- `fnm` for Node.js
- `Rust` and `cargo`
- `Homebrew for Linux`
- `Neovim` or `Helix`
- `Zellij`, `Zig`, `lazygit`, `opencode`
- `Ghostty` terminal
- `Chromium` and `Zen Browser`
- `Fcitx5` Chinese input
- `~/.gitconfig`
- Autologin to desktop session

Post-bootstrap hooks run after all steps complete. Add them under `bootstrap.hooks` in `vmctl.yaml`.

## Rebuild

```bash
rm -rf ~/.config/agent-vm/void-dev
go run ./cmd/agent-vm start
```

## Troubleshooting

- Log: `~/.config/agent-vm/void-dev/vfkit.log`
- Serial: `~/.config/agent-vm/void-dev/serial.log`
- Build log: `~/.config/agent-vm/void-dev/build-script.log`

## E2E Test

```bash
./scripts/e2e-test.sh
```

## Code Layout

```
cmd/agent-vm/main.go              CLI entry point
internal/vmctl/
  config.go                    config loading (LoadConfig/SaveConfig)
  yaml_config.go               YAML schema and parsing
  vm.go                        VM lifecycle (start/stop/destroy/bootstrap)
  build_vfkit.go               vfkit-based Void disk builder
  util.go                      shared helpers
  bootstrap_script.go          guest bootstrap script generator
  web.go / web_handlers.go     Echo v5 web server + REST API
  sync_config.go / sync_*.go   sync pair management
  tunnel_config.go / tunnel_*.go  SSH tunnel management
web/static/                    vanilla HTML/CSS/JS frontend
scripts/
  guest-bootstrap.sh           standalone guest bootstrap (reference)
  e2e-test.sh                  end-to-end test
```

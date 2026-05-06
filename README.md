# Agent VM

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A single-command, reproducible `arm64` Void Linux development VM on Apple Silicon macOS — managed entirely from a web UI.

- Distribution: `Void Linux aarch64 glibc`
- Desktop: configurable, default `Sway`
- Network: fixed IP `192.168.64.10`, gateway `192.168.64.1`
- Default user: `vm`
- Resources: `6 CPU / 6 GiB RAM / 100 GiB disk`

## Host Requirements

**Platform:** Apple Silicon macOS only. `vfkit` uses Apple's Virtualization framework.

```bash
vfkit
qemu-img
curl
ssh
go
```

Optional fallback disk builder: `podman`.

_Windows/Linux support would require replacing vfkit with QEMU. The disk image, kernel, and initrd are platform-agnostic — only the hypervisor layer needs changing._

Quick check:

```bash
command -v vfkit qemu-img curl ssh go
```

## Install

```bash
go install github.com/vrealzhou/agent-vm/cmd/agent-vm@latest
```

## Usage

The web UI is the primary interface — everything from first boot to daily management:

```bash
agent-vm             # open http://localhost:8080
agent-vm -p 9090     # custom port
```

The web UI is embedded in the binary. From it you can:

- **Bootstrap**: configure shell, editor, window manager, brew/cargo packages, and post-bootstrap hooks
- **VM control**: start, stop, destroy with live progress streaming and guest resource metrics
- **Sync**: set up file sync pairs (rsync or git) between host and VM
- **Tunnels**: manage SSH port forwarding

After bootstrap completes, the VM boots into the configured desktop session. Connect via SSH:

```bash
ssh vm@192.168.64.10
```

## Configuration

All config lives in `~/.config/agent-vm/vmctl.yaml`. Override the directory with `VMCTL_CONFIG_DIR`. Every key is optional — defaults apply for anything omitted.

```yaml
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

## CLI Commands

```bash
agent-vm start       # create assets + boot VM
agent-vm stop        # stop the VM
agent-vm destroy     # stop + remove VM state
agent-vm status      # VM state, PID, IP, disk path
agent-vm ssh         # SSH into guest as user "vm"
agent-vm ip          # print or set guest IP
agent-vm bootstrap   # run bootstrap flow
agent-vm sync        # manage sync pairs
agent-vm tunnel      # manage SSH tunnels
agent-vm help        # show all commands
```

## Networking

The VM runs behind vfkit NAT. Fixed IP `192.168.64.10/24`, gateway `192.168.64.1`. The host is reachable from inside the guest as `host.vm` — resolves to the gateway IP:

```bash
curl http://host.vm:8080/api/status
```

## Guest Bootstrap

Bootstrap configures inside the VM automatically on first boot:

- `fish` or `zsh` shell with `starship` prompt
- `fnm` for Node.js, `Rust` and `cargo`
- `Homebrew for Linux`, `Neovim` or `Helix`
- `Zellij`, `Zig`, `lazygit`, `opencode`
- `Ghostty` terminal, `Chromium`, `Zen Browser`
- `Fcitx5` Chinese input
- `~/.gitconfig`, autologin to desktop session

Post-bootstrap hooks run after all steps complete. Add them under `bootstrap.hooks` in `vmctl.yaml`.

## Rebuild

```bash
rm -rf ~/.config/agent-vm/void-dev
agent-vm start
```

## Troubleshooting

- Log: `~/.config/agent-vm/void-dev/vfkit.log`
- Serial: `~/.config/agent-vm/void-dev/serial.log`
- Build log: `~/.config/agent-vm/void-dev/build-script.log`

### VPN breaks SSH to VM

Cisco AnyConnect and similar VPNs redirect all traffic including local subnets:

```bash
BRIDGE=$(ifconfig | grep -B1 "192.168.64" | head -1 | awk '{print $1}' | sed 's/:$//')
sudo route -n add -net 192.168.64.0/24 -interface "$BRIDGE"
```

## Code Layout

```
cmd/agent-vm/main.go              CLI entry point
internal/vmctl/
  config.go / yaml_config.go      config loading and YAML schema
  vm.go / build_vfkit.go          VM lifecycle and disk builder
  util.go / bootstrap_script.go   helpers and bootstrap generator
  web.go / web_handlers.go        Echo v5 web server + REST API
  sync_*.go / tunnel_*.go         sync pair and tunnel management
web/static/                       embedded web UI
scripts/e2e-test.sh               end-to-end test
```

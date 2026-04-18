# vfkit Linux Dev VM Spec

Updated: 2026-04-18

## 1. Goal

Provide a single-command, reproducible `arm64` Linux development VM on Apple Silicon macOS using `vfkit`.

The project now maintains exactly one default path:

- Distribution: `Void Linux aarch64 glibc`
- Desktop: `Sway`
- Fixed IP: `192.168.64.10`
- SSH target: `ssh dev@192.168.64.10`
- Default resources: `6 CPU / 6 GiB RAM / 100 GiB disk`
- Entry point: `go run ./cmd/vmctl start`

Expected outcome:

- first boot downloads, builds, boots, and bootstraps automatically
- later boots reuse the existing VM state
- the codebase stays narrow, explicit, and rebuildable

## 2. Project Boundary

### 2.1 Supported Inputs

The project supports only:

1. The official Void `ROOTFS tarball`
2. Existing disk images: `.img`, `.img.xz`, `.raw`, `.raw.xz`, `.qcow2`

The default implementation is:

- download the official `ROOTFS`
- build the disk offline on the host
- extract `vmlinuz` and `initramfs`
- boot with `vfkit --kernel/--initrd`

### 2.2 Explicitly Unsupported Paths

These paths are intentionally not maintained:

- installer ISO
- cloud-init
- interactive installers
- board-firmware-oriented ARM images

Reason:

- they increase code and maintenance cost
- they do not improve the current default workflow
- this project is not trying to be a generic VM installer

## 3. Boot And System Behavior

### 3.1 First Boot

Running:

```bash
go run ./cmd/vmctl start
```

must automatically:

1. download the official Void rootfs
2. build a raw disk
3. write users, networking, SSH, GUI, and system config offline
4. extract `vmlinuz` and `initramfs`
5. start the VM
6. wait for SSH and run bootstrap once

### 3.2 Later Boots

If the disk and boot assets already exist:

- boot directly into the existing VM
- do not rerun bootstrap

Bootstrap completion is tracked by a `bootstrap.done` marker.

## 4. Users, Login, And GUI

### 4.1 Default Accounts

- Development user: `dev`
- Root user: `root`
- Default local passwords:
  - `dev / dev`
  - `root / root`

### 4.2 SSH

Requirements:

- reuse the host SSH public key by default
- inject the key into both `dev` and `root`
- allow:

```bash
ssh dev@192.168.64.10
```

### 4.3 GUI Session

Current GUI model:

- `tty1 autologin -> dev -> sway`

Requirements:

- boot directly into the `dev` `sway` session
- do not depend on a standalone display manager
- keep GUI and interactive SSH on the same `XDG_RUNTIME_DIR`

## 5. Networking

Default network model:

- `vfkit` NAT
- guest fixed IP: `192.168.64.10/24`
- default gateway: `192.168.64.1`

Access model:

- host to guest:
  - `ssh dev@192.168.64.10`
  - `http://192.168.64.10:<port>`
- guest to host:
  - `http://192.168.64.1:<port>`

The fixed IP configuration must bind to the virtual NIC MAC address rather than interface name.

## 6. Guest Software Set

### 6.1 Base System

- `linux6.12`
- `dracut`
- `NetworkManager`
- `dbus`
- `openssh`
- `curl`
- `wget`
- `git`
- `sudo`
- `chrony`

### 6.2 GUI And Desktop

- `sway`
- `seatd`
- `ghostty`
- `wofi`
- `mako`
- `grim`
- `slurp`
- `wl-clipboard`
- `xdg-desktop-portal-wlr`
- `mesa`
- `mesa-dri`

### 6.3 Browsers

- `Chromium`
- `Zen Browser`

Constraints:

- `Chromium` should default to an `Xwayland` wrapper
- `Zen Browser` should default to native `Wayland`

### 6.4 Input Method And Fonts

- `fcitx5`
- `fcitx5-chinese-addons`
- `fcitx5-configtool`
- `fcitx5-gtk+2`
- `fcitx5-gtk+3`
- `fcitx5-gtk4`
- `fcitx5-qt5`
- `fcitx5-qt6`
- `noto-fonts-cjk`
- `noto-fonts-emoji`

Default behavior:

- profile preloads `keyboard-us + pinyin`
- left `Shift` toggles the IME
- `Caps Lock` is a fallback toggle
- `swaybar` shows the current IME state

### 6.5 Development Environment

- `fish`
- `oh-my-posh`
- `neovim`
- `rustup`
- `Homebrew for Linux`
- `helix`
- `zellij`
- `zig`
- `opencode`
- `lazygit`
- `gitui`
- `fresh-editor`

Constraints:

- `helix`, `zellij`, `zig`, `opencode`, `lazygit`, and `gitui` should default to Linux Homebrew
- `fresh-editor` should install through Cargo
- Cargo packages must support `crate:command` validation
- if the command already exists, bootstrap must skip reinstalling it

## 7. Shell, Git, And Time

### 7.1 Fish And Oh My Posh

Requirements:

- default shell: `fish`
- default prompt theme: `unicorn`
- true-color shell support enabled

### 7.2 Git

Bootstrap must initialize `~/.gitconfig` with at least:

- `core.editor = nvim`
- `init.defaultBranch = main`
- `push.autoSetupRemote = true`
- `rebase.autoStash = true`
- `merge.conflictstyle = zdiff3`

It must also support:

- `VM_GIT_USER_NAME`
- `VM_GIT_USER_EMAIL`

### 7.3 Time

Requirements:

- default timezone: `Australia/Sydney`
- automatic time sync enabled
- current implementation uses `chronyd`

## 8. Clipboard

The goal is not seamless system-level clipboard sync. The goal is stable helper-based sharing.

Supported commands:

- `vmctl clip-in`
- `vmctl clip-out`

Requirements:

- the guest is already in `Sway`
- the current Wayland session is available

Explicitly not promised:

- host/guest system-level seamless sync
- any solution that depends on `SPICE` or `spice-vdagent`

## 9. Customization Model

All common overrides should be controlled through a repo-root `.vmctl.env`.

Typical override groups:

- resources: `VM_CPUS`, `VM_MEMORY_MIB`, `VM_DISK_SIZE`
- networking: `VM_STATIC_IP`, `VM_GATEWAY`, `VM_DNS_SERVERS`, `VM_MAC`
- accounts: `VM_GUEST_USER`, `VM_GUEST_PASSWORD`, `VM_ROOT_PASSWORD`
- SSH: `VM_SSH_PUBLIC_KEY`, `VM_SSH_KNOWN_HOSTS_FILE`
- image source: `VM_BASE_IMAGE`, `VM_BASE_IMAGE_URL`
- display: `VM_WIDTH`, `VM_HEIGHT`
- bootstrap packages: `VM_BOOTSTRAP_BREW_PACKAGES`, `VM_BOOTSTRAP_CARGO_PACKAGES`
- git identity: `VM_GIT_USER_NAME`, `VM_GIT_USER_EMAIL`

## 10. Acceptance Criteria

The current solution is considered valid if all of the following hold:

1. `go run ./cmd/vmctl start` completes download, disk build, and boot
2. the GUI automatically enters the `dev` `sway` session
3. the host can `ssh dev@192.168.64.10`
4. the host can access guest HTTP services
5. `bootstrap.done` prevents an unexpected second bootstrap
6. `fish`, `Ghostty`, `Rust`, `Homebrew`, `Helix`, `Zellij`, `Zig`, `Chromium`, `Zen Browser`, and `Fcitx5` are all present

## 11. Current Implementation Constraints

- the default path depends on `podman` to build the Void disk on the host
- `Chromium` Chinese input is optimized for compatibility, so it defaults to `Xwayland`
- cross-VM clipboard is helper-based, not system-integrated

## 12. References

- Void Linux live images:
  [https://docs.voidlinux.org/installation/live-images/index.html](https://docs.voidlinux.org/installation/live-images/index.html)
- Void live/current index:
  [https://repo-default.voidlinux.org/live/current/](https://repo-default.voidlinux.org/live/current/)
- Void aarch64 package index:
  [https://repo-default.voidlinux.org/current/aarch64/](https://repo-default.voidlinux.org/current/aarch64/)
- vfkit usage:
  [https://github.com/crc-org/vfkit/blob/main/doc/usage.md](https://github.com/crc-org/vfkit/blob/main/doc/usage.md)

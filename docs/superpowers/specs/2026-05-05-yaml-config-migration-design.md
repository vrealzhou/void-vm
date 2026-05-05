# YAML Config Migration Design

Updated: 2026-05-05

## 1. Goal

Consolidate vmctl configuration, runtime state, and generated scripts under a single directory `~/.config/agent-vm/`. Replace the repo-local `.vmctl.env` (KEY=VALUE), `.vmctl.sync` (JSON), and `.vmctl.tunnels` (JSON) with one YAML config file. Improve bootstrap package specification from ad-hoc strings to structured YAML. Add post-bootstrap hook support.

## 2. Directory Layout

```
~/.config/agent-vm/            # VMCTL_CONFIG_DIR (env-overridable)
├── vmctl.yaml                 # all config: VM, bootstrap, sync, tunnels
├── scripts/
│   └── guest-bootstrap.sh     # generated from Go template at bootstrap time
├── images/                    # downloaded Void rootfs tarballs or custom disk images
└── void-dev/                  # runtime state (was .vm/void-dev/)
    ├── disk.img
    ├── vmlinuz
    ├── initramfs.img
    ├── bootstrap.done
    ├── vfkit.log
    ├── serial.log
    ├── vfkit.pid
    └── efi-vars.fd
```

- `VMCTL_CONFIG_DIR` env var overrides the base directory. Default: `~/.config/agent-vm/`.
- If `VMCTL_CONFIG_DIR` is set, the entire tree shifts under it (yaml, scripts, images, void-dev).
- vmctl creates `scripts/`, `images/`, and `void-dev/` automatically on first run.
- The repo-local `scripts/guest-bootstrap.sh` remains as a standalone reference but is no longer the canonical copy.

## 3. YAML Config Schema

Every key is optional. Omitted keys get Go-level defaults from the Config struct.

```yaml
# ~/.config/agent-vm/vmctl.yaml

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
    guest_path: /home/vm/myproject
    mode: copy
    direction: host-to-vm       # host-to-vm | vm-to-host | bidirectional (default: host-to-vm)
    exclude: [node_modules, .git]
    exclude_from: ""
    backup_retention_days: 7

tunnels:
  - name: webapp
    type: local                  # local | remote
    local_port: 3000
    remote_host: localhost
    remote_port: 3000
    enabled: true
    auto_start: true
```

### 3.1 Field Defaults

| Section | Key | Default |
|---------|-----|---------|
| vm | name | `void-dev` |
| vm | cpus | `6` |
| vm | memory_mib | `6144` |
| vm | disk_size | `"100G"` |
| vm | gui | `true` |
| vm | width | `1920` |
| vm | height | `1200` |
| network | static_ip | `"192.168.64.10"` |
| network | gateway | `"192.168.64.1"` |
| network | cidr | `24` |
| network | dns_servers | `["1.1.1.1", "8.8.8.8"]` |
| network | mac | `"52:54:00:64:00:10"` |
| user | name | `vm` |
| user | password | `dev` |
| user | root_password | `root` |
| user | ssh_public_key | `""` → auto-detect `~/.ssh/id_ed25519.pub` |
| guest | timezone | `Australia/Sydney` |
| guest | default_shell | `fish` |
| guest | default_editor | `neovim` |
| guest | window_manager | `sway` |
| bootstrap | brew_packages | `[]` |
| bootstrap | cargo_packages | `[]` |
| bootstrap | hooks | `[]` |
| git | user_name | `""` |
| git | user_email | `""` |
| sync | (list) | `[]` |
| tunnels | (list) | `[]` |

### 3.2 Bootstrap Packages

- **brew_packages**: list of Homebrew formula names. Each is passed directly to `brew install`.
- **cargo_packages**: list of objects with `crate` (Cargo crate name) and `command` (executable checked before install). If `command` is omitted, defaults to `crate`.

### 3.3 Post-Bootstrap Hooks

- **hooks**: list of inline shell command strings. Appended to the end of the generated guest bootstrap script, after the main `main()` function. Each hook is executed only if all preceding bootstrap steps succeeded. Hooks run as the target user inside the guest.
- Hooks execute only once, as part of bootstrap. They do not run on subsequent VM restarts. To re-run hooks, run `vmctl bootstrap`.

### 3.4 Sync Entries

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| name | string | — | Unique identifier (auto-generates `id`) |
| host_path | string | — | Absolute path on host |
| guest_path | string | — | Absolute path in guest |
| mode | string | `copy` | `copy` (rsync) or `git` |
| direction | string | `host-to-vm` | `host-to-vm`, `vm-to-host`, or `bidirectional` |
| exclude | []string | `[]` | Paths to exclude from sync |
| exclude_from | string | `""` | Path to an exclude file (e.g., `.gitignore`) |
| backup_retention_days | int | `7` | Days to retain backups for copy-mode syncs |

### 3.5 Tunnel Entries

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| name | string | — | Unique identifier (auto-generates `id`) |
| type | string | `local` | `local` (host→guest) or `remote` (guest→host) |
| local_port | int | — | Port on host |
| remote_host | string | `localhost` | Target host inside the VM |
| remote_port | int | — | Port on guest |
| enabled | bool | `true` | Whether the tunnel is active |
| auto_start | bool | `true` | Auto-start tunnel on VM boot |

## 4. Config Loading

- `LoadConfig()` reads `$VMCTL_CONFIG_DIR/vmctl.yaml` (default `~/.config/agent-vm/`).
- Uses `gopkg.in/yaml.v3` for parsing.
- Missing keys → Go struct defaults from the Config struct.
- No fallback to `.vmctl.env`, `.vmctl.sync`, or `.vmctl.tunnels` — clean break.
- If `vmctl.yaml` does not exist, vmctl creates a defaults-commented template on first run.
- `sync` and `tunnels` sections are loaded into the same in-memory types currently used by `sync_config.go` and `tunnel_config.go`.

## 5. SSH Key Resolution

- `user.ssh_public_key` blank or unset → look for `~/.ssh/id_ed25519.pub`.
- Set to an absolute path → use that file.
- Private key derived from public key path (strip `.pub` suffix).
- If the resolved public key file does not exist, vmctl errors with a clear message.

## 6. Image Handling

- Base images always live under `~/.config/agent-vm/images/`. Not configurable.
- Latest Void Linux aarch64 ROOTFS URL auto-fetched from `https://repo-default.voidlinux.org/live/current/`. No `base_image_url` config key.
- To use a custom disk image (`.img`/`.raw`/`.qcow2`), place it in `images/`. vmctl detects and uses it instead of downloading a rootfs tarball.

## 7. Bootstrap Script Generation

- `bootstrap_script.go` template logic stays the same.
- Generated script is written to `~/.config/agent-vm/scripts/guest-bootstrap.sh` at bootstrap time.
- `bootstrap.brew_packages` and `bootstrap.cargo_packages` are injected into the script as parsed YAML arrays (replacing the current `BOOTSTRAP_BREW_PACKAGES` / `BOOTSTRAP_CARGO_PACKAGES` env-var string parsing in the script).
- `bootstrap.hooks` are appended to the script as a final step, wrapped in error checking.
- The repo-local `scripts/guest-bootstrap.sh` stays as a reference copy but is not the runtime script path.

## 8. Sync & Tunnels Migration

- Current `.vmctl.sync` and `.vmctl.tunnels` JSON files at repo root are no longer used.
- Web UI API handlers (`/api/sync/*`, `/api/tunnels/*`) read/write the `sync` and `tunnels` sections of `vmctl.yaml` instead.
- Existing sync/tunnel data is not auto-migrated. Users re-add entries in the YAML file.

## 9. Web UI Changes

- Templates/form updates in `web/static/` to reflect new config structure (brew list, cargo list, hooks).
- Config editor / bootstrap popup reads from and writes to `vmctl.yaml`.

## 10. Removed Configuration Keys

These env vars from the old `.vmctl.env` have no YAML equivalent:

| Old key | Reason |
|---------|--------|
| `VM_SSH_USER` | Merged into `user.name` |
| `VM_GUEST_USER` | Merged into `user.name` |
| `VM_SSH_KNOWN_HOSTS_FILE` | Always uses `~/.ssh/known_hosts` |
| `VM_IMAGE_DIR` | Fixed under `~/.config/agent-vm/images/` |
| `VM_BASE_IMAGE_URL` | Auto-fetched latest; no config needed |
| `VM_VOID_REPOSITORY` | Always uses default Void repo |
| `VM_STATE_DIR` | Fixed under `<config_dir>/void-dev/` |
| `VM_BOOTSTRAP_BREW_PACKAGES` | Replaced by `bootstrap.brew_packages` list |
| `VM_BOOTSTRAP_CARGO_PACKAGES` | Replaced by `bootstrap.cargo_packages` list |

## 11. Test Plan

- New `internal/vmctl/yaml_config_test.go` — YAML parsing, default resolution, missing file handling
- Update `internal/vmctl/config_test.go` — replace env-var tests with YAML tests
- Update `internal/vmctl/sync_config_test.go` — use YAML-backed config
- Update `internal/vmctl/tunnel_config_test.go` — use YAML-backed config
- New `internal/vmctl/bootstrap_script_test.go` — verify brew/cargo/hooks injected into generated script
- Update `scripts/e2e-test.sh` — use new paths and YAML config
- Update `web/` UI to work with new config types

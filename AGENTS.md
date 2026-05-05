# AGENTS.md

## Build & Run

```bash
go run ./cmd/vmctl          # default: start web UI on port 8080
go run ./cmd/vmctl start    # CLI: create assets + boot VM
go run ./cmd/vmctl status   # VM state, PID, IP, disk path
go run ./cmd/vmctl ssh      # SSH into guest as user "vm" @ 192.168.64.10
```

No Makefile. Run the binary directly. Module: `github.com/vrealzhou/agent-vm`, Go 1.26.

## Test

```bash
go test ./internal/vmctl/...             # all unit tests
go test -short ./internal/vmctl/...      # skip tests needing cpio/tar/xz binaries
./scripts/e2e-test.sh                    # full integration: boots VM, verifies SSH, chromium, bootstrap
```

E2E test requires `vfkit`, `qemu-img`, `ssh`, `podman`, and an `id_ed25519` key. It runs in `.cache-test/e2e/`.

## Architecture

```
cmd/vmctl/main.go          # CLI entrypoint
internal/vmctl/            # all Go code (config.go, yaml_config.go, vm.go, util.go, build_vfkit.go, bootstrap_script.go, web*.go, sync*.go, tunnel*.go)
web/static/                # vanilla HTML/CSS/JS frontend (served by echo/v5)
scripts/                   # guest-bootstrap.sh (reference), e2e-test.sh
docs/                      # specs.md + superpowers design docs
```

- CLI framework: `spf13/cobra` (see `cobra.go` for subcommand registration)
- Web server: `labstack/echo/v5` on `VM_MANAGER_PORT` (default 8080), serves `web/static/` + REST API
- Config: `~/.config/agent-vm/vmctl.yaml` (YAML). Override with `VMCTL_CONFIG_DIR`
- VM lifecycle: `vm.go` — `Start()`, `Stop()`, `Destroy()`, `Bootstrap()`
- Disk building: `build_vfkit.go` — launches a transient build VM via vfkit (fallback: podman)
- Guest bootstrap: `bootstrap_script.go` generates a shell script from Go template; written to `~/.config/agent-vm/scripts/guest-bootstrap.sh` at bootstrap time
- Sync: sync pairs stored in `vmctl.yaml` under `sync:` section; `sync_copy.go` (rsync), `sync_git.go` (git push/pull)
- Tunnels: tunnels stored in `vmctl.yaml` under `tunnels:` section; `tunnel_manager.go` (SSH -L/-R via PID files)

## Conventions & Gotchas

- Runtime state lives under `~/.config/agent-vm/void-dev/`: `disk.img`, `vmlinuz`, `initramfs.img`, `bootstrap.done`, `vfkit.log`, `serial.log`, `vfkit.pid`, `efi-vars.fd`
- Base images in `~/.config/agent-vm/images/`
- Default VM: 6 CPUs, 6144 MiB RAM, 100G disk, fixed IP 192.168.64.10
- Default user: `vm` / password: `dev`; root: `root` / password: `root`
- SSH key auto-detected from `~/.ssh/id_ed25519.pub`; override via `user.ssh_public_key` in YAML
- Two supported input types: Void rootfs tarball or existing disk image (.img/.raw/.qcow2)
- No lint/formatter config; no CI; no pre-commit hooks

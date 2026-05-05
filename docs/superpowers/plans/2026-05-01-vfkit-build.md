# Replace Podman With VFKit Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` for parallel exploration and implementation, or `superpowers:executing-plans` when working step-by-step. Keep checkbox (`- [ ]`) status updated as each step completes.

## Goal

Replace the current Podman-based Void Linux disk builder with a VFKit-based builder, while preserving the existing runtime behavior:

- `go run ./cmd/vmctl` opens the GUI control panel.
- `Bootstrap` is the first-time guided setup flow.
- `bootstrap.done` gates `Start` and `Stop` in the GUI.
- Node.js always uses `fnm`.
- Existing shell/editor/window-manager preferences remain `fish|zsh`, `neovim|helix`, and `sway|xfce`.
- Existing sync, tunnel, web, progress, and kernel upgrade helpers keep working.

## Current State

The current rootfs build path is `buildVoidLinuxDisk(cfg)` in `internal/vmctl/vm.go`. It runs a Debian container through Podman, unpacks the Void rootfs tarball, installs packages through `xbps`, writes system configuration, extracts `vmlinuz` and `initramfs.img`, and leaves a raw disk at `cfg.DiskPath`.

The replacement must keep the public call site stable:

```go
func buildVoidLinuxDisk(cfg Config) error
```

The implementation may move into a new file, but `prepareDisk`, `createDiskFromBaseImage`, GUI bootstrap, and CLI commands should not need a new user-facing workflow.

## Target Architecture

The host builds a temporary VFKit build VM:

1. Download and extract a Void kernel package on the host.
2. Extract the Void rootfs tarball into a temporary build rootfs.
3. Inject a purpose-built executable `/init`.
4. Inject build-time SSH and network configuration.
5. Create a `newc` cpio gzip initrd from that build rootfs.
6. Create a temporary target raw disk, not `cfg.DiskPath` directly.
7. Boot VFKit with the build kernel, build initrd, and target disk.
8. Let `/init` configure networking and either run the full build locally or start root SSH for host-driven setup.
9. Install the target disk system, generate boot assets, and signal completion.
10. Copy or move the finished disk and boot assets into the normal state directory.
11. Stop the build VM and clean up temporary files.

The first implementation should prefer one of these two execution models:

- `init-local`: `/init` runs the entire build script inside the build VM and writes completion to serial.
- `root-ssh`: `/init` starts sshd for `root`, then the host runs the setup script over SSH.

Do not use `cfg.SSHUser` for the build VM. The build VM should use an internal root login with the host public key, because the normal guest user may not exist until after the target disk is configured.

## Non-Goals

- Do not rework the GUI, web UI, sync, tunnel, or bootstrap preference flow.
- Do not remove `BootstrapSetup`; the VFKit disk builder only replaces the offline disk build.
- Do not try to install Homebrew, `fnm`, Starship, or user dotfiles in the build initrd unless the current offline disk builder already does that exact work. First boot bootstrap remains responsible for guest user tooling.
- Do not write directly to `cfg.DiskPath` until the build has succeeded.

## Task 1: Add Build Kernel Configuration

**Files:**

- Modify: `internal/vmctl/config.go`
- Modify: `.vmctl.env.example`
- Modify: `README.md`

- [ ] Add `BuildKernelURL string` to `Config`.
- [ ] Load it from `VM_BUILD_KERNEL_URL`.
- [ ] Default to a Linux package URL under `https://repo-default.voidlinux.org/current/aarch64/`.
- [ ] Document that the URL must point to an aarch64 Void Linux kernel `.xbps` package.
- [ ] Avoid pinning the plan to a stale package version without an update note. If pinning is necessary, document the exact date the package was verified.

Verification:

```bash
go test ./internal/vmctl/...
```

## Task 2: Implement Kernel Package Extraction

**Files:**

- Create or modify: `internal/vmctl/build_vfkit.go`

- [ ] Add `downloadBuildKernel(cfg Config) (string, error)`.
- [ ] Download the kernel package into `cfg.StateDir`.
- [ ] Extract with `tar -xf`; Void `.xbps` packages are tar archives.
- [ ] Find the kernel under `boot/` with names matching `vmlinux-*`, `vmlinuz-*`, or `Image-*`.
- [ ] Copy it to `cfg.StateDir/build-vmlinuz`.
- [ ] Return useful errors when no kernel is found.
- [ ] Add a unit test that extracts from a small synthetic tar archive and finds the expected kernel path.

Implementation notes:

- Use existing `ensureDownloadedFile`, `copyFile`, `fileExists`, and `addProgress`.
- Do not assume GNU tar-only flags.

Verification:

```bash
go test ./internal/vmctl/...
```

## Task 3: Create a Real Build Initrd With `/init`

**Files:**

- Create or modify: `internal/vmctl/build_vfkit.go`
- Add tests in: `internal/vmctl/build_vfkit_test.go`

- [ ] Add `prepareBuildRootfs(cfg Config, rootfsDir string) error`.
- [ ] Extract `cfg.BaseImage` into `rootfsDir`.
- [ ] Inject an executable `/init`.
- [ ] Inject root SSH authorized keys for the build VM.
- [ ] Inject DNS and repository config.
- [ ] Ensure the build rootfs contains the commands required by `/init`, or fail early with a clear error.
- [ ] Add `createCpioInitrd(rootfsDir, outputPath string) error`.
- [ ] Use `find . | cpio -o -H newc | gzip -1` semantics without leaking open file descriptors.
- [ ] Add tests that assert the generated cpio contains `init` and that `/init` is executable.

Required `/init` behavior:

- Mount `/proc`, `/sys`, `/dev`, and `/run`.
- Bring up `eth0` using the configured static IP and gateway, or DHCP if the implementation explicitly chooses DHCP.
- Write `/etc/resolv.conf`.
- Start `sshd` for root if using the `root-ssh` model.
- Run the local build script if using the `init-local` model.
- Write clear progress messages to `/dev/hvc0` or the configured serial console.
- Power off or wait in a debuggable shell after failure.

Important correction from the previous plan:

- Do not boot with `rdinit=/init` unless `/init` exists and is executable.
- Do not wait for SSH unless `/init` has installed or started sshd and root login is configured.

Verification:

```bash
go test ./internal/vmctl/...
```

Manual smoke test:

```bash
go run ./cmd/vmctl vfkit-build-smoke
```

If no temporary CLI command is added, document the exact command used to boot the smoke-test initrd.

## Task 4: Prove the Build VM Boots Before Writing the Real Disk

**Files:**

- Create or modify: `internal/vmctl/build_vfkit.go`
- Optionally modify: `internal/vmctl/cobra.go`

- [ ] Add an internal smoke-test helper that boots VFKit with the build kernel and build initrd.
- [ ] Use a temporary disk under `os.MkdirTemp`, not `cfg.DiskPath`.
- [ ] Use a separate temporary PID file, REST socket, log file, and serial log.
- [ ] Always stop the build VM with `defer Stop(buildCfg)` or equivalent cleanup.
- [ ] Wait for `VirtualMachineStateRunning`.
- [ ] Confirm either serial success output or root SSH readiness.
- [ ] Surface the serial log tail in returned errors.

Build VM configuration rules:

- Use a separate MAC address from the normal VM or document why reuse is safe.
- Use a separate static IP from the normal VM if the normal VM may be running.
- Use root SSH args for the build VM.
- Do not call `waitForSSH(buildCfg, buildCfg.SSHUser, ...)`.

Verification:

```bash
go test ./internal/vmctl/...
```

Manual verification:

```bash
go run ./cmd/vmctl vfkit-build-smoke
```

Expected result: VFKit reaches running state and the smoke signal is visible over serial or root SSH.

## Task 5: Implement Target Disk Build Inside the Build VM

**Files:**

- Modify: `internal/vmctl/build_vfkit.go`

- [ ] Create the target disk at a temporary path, for example `cfg.DiskPath + ".building"`.
- [ ] Attach it to the build VM as `virtio-blk`.
- [ ] Format it as ext4.
- [ ] Copy the build rootfs into the mounted target disk while excluding live mounts and temporary build files.
- [ ] Chroot into the target disk.
- [ ] Install or reconfigure the same base package set currently installed by `voidLinuxBuildScript()`.
- [ ] Generate `/boot` assets with `xbps-reconfigure` or a narrower equivalent.
- [ ] Copy kernel and initramfs out through existing `copyRemoteFile` or direct host-side disk mount only if that is implemented and tested.
- [ ] Rename the temporary disk to `cfg.DiskPath` only after kernel, initrd, and disk have all succeeded.

Preserve these current behaviors:

- Static networking must use NetworkManager keyfile format `address1=IP/CIDR,GATEWAY`.
- Default user, passwords, sudoers, SSH key, timezone, chrony, and runit services must match current behavior.
- Shell choice must support `fish` and `zsh`.
- Window manager choice must support `sway` and `xfce`.
- Desktop autologin and session startup must match the current implementation.
- The build must continue to produce `cfg.KernelPath` and `cfg.InitrdPath` for direct kernel boot.

Verification:

```bash
go test ./internal/vmctl/...
```

Manual verification:

```bash
rm -rf .vm/void-dev
go run ./cmd/vmctl bootstrap
go run ./cmd/vmctl status
```

Expected result: disk build succeeds without Podman, the VM boots, SSH works, and `bootstrap.done` is written.

## Task 6: Wire the VFKit Builder Into the Existing Build Path

**Files:**

- Modify: `internal/vmctl/vm.go`
- Modify: `README.md`
- Modify: `docs/specs.md`

- [ ] Replace only the internals of `buildVoidLinuxDisk(cfg)`.
- [ ] Keep the function name and callers stable.
- [ ] Remove the `exec.LookPath("podman")` requirement.
- [ ] Keep `qemu-img` requirements for non-rootfs image conversion paths.
- [ ] Update help text that currently says rootfs build depends on Podman.
- [ ] Add `VM_BUILD_KERNEL_URL` to usage and docs.
- [ ] Remove Podman from host requirements once the VFKit path is proven.

Verification:

```bash
go build ./cmd/vmctl
go test ./internal/vmctl/...
```

## Task 7: Protect Against Regressions

**Files:**

- Modify: `scripts/e2e-test.sh`
- Add tests under: `internal/vmctl`

- [ ] Add tests for cpio initrd creation.
- [ ] Add tests for build kernel extraction.
- [ ] Add tests for NetworkManager keyfile generation if it is factored into a helper.
- [ ] Add tests for build VM config using root SSH instead of `cfg.SSHUser`.
- [ ] Update E2E to assert Podman is not required for rootfs builds.
- [ ] Keep existing checks for `fnm`, Starship, Fish config, and `bootstrap.done`.

Manual E2E:

```bash
./scripts/e2e-test.sh
```

Expected result: full bootstrap and restart path passes without Podman installed or running.

## Task 8: Commit

- [ ] Run final verification:

```bash
go build ./cmd/vmctl
go test ./...
bash -n scripts/guest-bootstrap.sh
bash -n scripts/e2e-test.sh
```

- [ ] Inspect the diff and confirm it does not remove current GUI/bootstrap/sync/tunnel behavior.
- [ ] Commit:

```bash
git add internal/vmctl .vmctl.env.example README.md docs/specs.md scripts/e2e-test.sh
git commit -m "feat: build Void rootfs with vfkit"
```

## Known Risks

- The Void rootfs tarball may not include every command required by the build initrd. The implementation must check for required commands before booting VFKit or make the `/init` path self-contained.
- macOS `cpio` and `tar` behavior should be tested directly; avoid GNU-only flags.
- The build VM can conflict with the normal VM if it reuses the same IP, MAC, PID file, or REST socket.
- Kernel package URLs change over time. `VM_BUILD_KERNEL_URL` must be configurable and documented.
- The VFKit path should not become a second divergent guest configuration system. Keep first-boot `BootstrapSetup` responsible for user-level tooling where possible.

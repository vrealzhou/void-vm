# Kernel Upgrade Feature

## Goal

Allow users to upgrade the VM kernel with one click from the Web UI, without rebuilding the entire disk image.

## Background

vfkit uses Direct Kernel Boot (`--kernel` + `--initrd`). The kernel and initrd are extracted during `buildVoidLinuxDisk` and stored on the host at `{StateDir}/vmlinuz` and `{StateDir}/initramfs.img`. After upgrading the kernel inside the VM via `xbps-install`, the boot assets on the host are stale and a reboot alone won't use the new kernel.

## Design

### API

`POST /api/upgrade-kernel`

Preconditions:
- VM must be running
- SSH must be reachable

Steps (run sequentially):
1. SSH: `xbps-install -uy linux6.12` (upgrade kernel package)
2. SSH: `xbps-reconfigure -f linux6.12` (regenerate initramfs)
3. SSH: find latest kernel and initramfs in `/boot/`
4. Pipe kernel and initramfs back to host via `ssh ... 'cat /boot/...' > StateDir/vmlinuz`
5. Stop VM
6. Start VM (now boots with new kernel)

Response: JSON `{ "message": "...", "kernelVersion": "..." }`

### Backend function

`UpgradeKernel(cfg Config) error` in `internal/vmctl/vm.go`:
1. `waitForSSH(cfg, cfg.SSHUser, 60s)`
2. Run xbps-install via SSH
3. Run xbps-reconfigure via SSH
4. Find kernel path: `ssh ... 'ls -1 /boot/vmlinux-* /boot/vmlinuz-* 2>/dev/null | sort | tail -1'`
5. Find initrd path: `ssh ... 'ls -1 /boot/initramfs-*.img 2>/dev/null | sort | tail -1'`
6. Copy kernel: `sshArgs + 'cat ' + kernelPath` → write to `cfg.KernelPath`
7. Copy initrd: same → `cfg.InitrdPath`
8. Stop VM
9. Start VM

### Web UI

- Add "Upgrade Kernel" button in Actions section
- Button disabled when VM not running or busy
- Shows progress in activity area
- On success: toast notification with new kernel version

### Status display

- Add kernel version to `InspectVM` response
- Extract from vmlinuz filename if boot assets exist, or SSH into VM
- Display in VM Status panel

### Error handling

- If xbps-install fails: return error, don't proceed
- If copy fails: return error, old kernel/initrd remain intact (write to temp file first, then rename)
- If VM restart fails: user can manually start from UI

### Atomic file writes

Write new kernel/initrd to temp files first, then rename:
1. Write to `cfg.KernelPath + ".new"`
2. Write to `cfg.InitrdPath + ".new"`
3. `os.Rename` both to final paths

## Existing fallback

Destroy + Bootstrap still works as the full-rebuild fallback. The Upgrade Kernel button is for incremental updates.

package vmctl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Start(cfg Config) error {
	if _, err := exec.LookPath("vfkit"); err != nil {
		return fmt.Errorf("missing required command: vfkit")
	}

	running, err := pidIsRunning(cfg.PIDFile)
	if err != nil {
		return err
	}
	if running {
		logf("VM is already running")
		return Status(cfg)
	}

	buildPIDFile := filepath.Join(cfg.StateDir, "build-vfkit.pid")
	buildRunning, err := pidIsRunning(buildPIDFile)
	if err == nil && buildRunning {
		return fmt.Errorf("disk build is already in progress (PID file: %s)", buildPIDFile)
	}

	voidBootstrapCandidate := isVoidLinuxRootfsTarball(cfg.BaseImage)

	cfg, err = prepareDisk(cfg)
	if err != nil {
		return err
	}
	_ = os.Remove(cfg.RestSocket)

	logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command("vfkit", vfkitArgs(cfg)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	logf("starting %s", cfg.Name)
	addProgress("launching vfkit...")
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()

	addProgress("waiting for VM to reach running state...")
	if err := waitForState(cfg, "VirtualMachineStateRunning", 90*time.Second); err != nil {
		tail, tailErr := tailFile(cfg.LogFile, 80)
		if tailErr == nil && tail != "" {
			fmt.Fprint(os.Stderr, tail)
		}
		return fmt.Errorf("vfkit did not reach running state")
	}

	logf("VM started")
	if voidBootstrapCandidate && !fileExists(cfg.BootstrapMarker) {
		logf("waiting for root SSH to fix guest configuration...")
		if err := waitForSSH(cfg, "root", 3*time.Minute); err != nil {
			return err
		}
		if err := fixGuestConfig(cfg); err != nil {
			logf("fix guest config: %v", err)
		}
		logf("waiting for SSH so first-boot bootstrap can finish")
		addProgress("waiting for VM SSH to become available...")
		if err := waitForSSH(cfg, cfg.SSHUser, 5*time.Minute); err != nil {
			addProgress("guest SSH for %s not ready, retrying root repair...", cfg.SSHUser)
			if repairErr := fixGuestConfig(cfg); repairErr != nil {
				logf("retry fix guest config: %v", repairErr)
			}
			if retryErr := waitForSSH(cfg, cfg.SSHUser, 90*time.Second); retryErr != nil {
				return retryErr
			}
		}
		addProgress("SSH available, running bootstrap...")
		if err := Bootstrap(cfg); err != nil {
			return err
		}
		if err := os.WriteFile(cfg.BootstrapMarker, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return err
		}
		addProgress("bootstrap complete")
	}
	if err := StartAutoTunnels(cfg); err != nil {
		logf("auto-start tunnels: %v", err)
	}
	return Status(cfg)
}

func Stop(cfg Config) error {
	running, err := pidIsRunning(cfg.PIDFile)
	if err != nil {
		return err
	}
	if !running {
		logf("VM is not running")
		return nil
	}

	logf("stopping %s", cfg.Name)
	if err := restStateChange(cfg, "HardStop"); err != nil {
		pid, readErr := readPID(cfg.PIDFile)
		if readErr == nil {
			if proc, findErr := os.FindProcess(pid); findErr == nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
		}
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		running, err = pidIsRunning(cfg.PIDFile)
		if err != nil {
			return err
		}
		if !running {
			_ = os.Remove(cfg.PIDFile)
			_ = os.Remove(cfg.RestSocket)
			logf("VM stopped")
			if err := StopAllTunnels(cfg); err != nil {
				logf("stop tunnels: %v", err)
			}
			return nil
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("VM did not stop in time")
}

func Status(cfg Config) error {
	status, err := InspectVM(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("name: %s\n", status.Name)
	fmt.Printf("state: %s\n", status.State)
	fmt.Printf("disk: %s\n", status.DiskPath)
	fmt.Printf("ip: %s\n", status.StaticIP)
	fmt.Printf("bootstrap: %t\n", status.BootstrapDone)
	if status.Running {
		fmt.Printf("pid: %d\n", status.PID)
		fmt.Printf("ssh: ssh %s\n", status.SSHTarget)
	}
	return nil
}

func SSH(cfg Config, extraArgs []string) error {
	args := sshArgs(cfg)
	args = append(args, extraArgs...)
	cmd := exec.Command("ssh", args...)
	return runWithSignals(cmd)
}

func Bootstrap(cfg Config) error {
	script, err := generateBootstrapScript(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate bootstrap script: %w", err)
	}

	scriptsDir := filepath.Join(cfg.ConfigDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create scripts dir: %w", err)
	}
	scriptPath := filepath.Join(scriptsDir, "guest-bootstrap.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("failed to write bootstrap script: %w", err)
	}

	logf("configuring %s + %s + %s inside %s", cfg.DefaultShell, cfg.DefaultEditor, cfg.WindowManager, cfg.Name)
	addProgress("running bootstrap script (this may take several minutes)...")

	cmd := exec.Command("ssh", append(sshArgsForUser(cfg, cfg.SSHUser), "bash -s")...)
	cmd.Stdin = strings.NewReader(script)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	scannerOut := bufio.NewScanner(stdout)
	scannerErr := bufio.NewScanner(stderr)
	go func() {
		for scannerOut.Scan() {
			addProgress("%s", scannerOut.Text())
		}
	}()
	go func() {
		for scannerErr.Scan() {
			addProgress("%s", scannerErr.Text())
		}
	}()
	if err := cmd.Wait(); err != nil {
		addProgress("bootstrap script failed: %v", err)
		return err
	}
	return nil
}

func BootstrapSetup(cfg Config) error {
	addProgress("starting bootstrap setup...")
	status, err := InspectVM(cfg)
	if err != nil {
		return err
	}

	if !status.Running {
		if err := Start(cfg); err != nil {
			return err
		}
		if fileExists(cfg.BootstrapMarker) {
			addProgress("VM started, bootstrap already done")
			return nil
		}
	}

	addProgress("waiting for SSH to run bootstrap...")
	if err := waitForSSH(cfg, cfg.SSHUser, 5*time.Minute); err != nil {
		addProgress("guest SSH for %s not ready, attempting root-side repair...", cfg.SSHUser)
		if repairErr := fixGuestConfig(cfg); repairErr != nil {
			logf("fix guest config: %v", repairErr)
		}
		if retryErr := waitForSSH(cfg, cfg.SSHUser, 90*time.Second); retryErr != nil {
			return retryErr
		}
	}
	addProgress("running bootstrap script...")
	if err := Bootstrap(cfg); err != nil {
		return err
	}
	addProgress("bootstrap setup complete")
	return writeBootstrapMarker(cfg)
}

func UpgradeKernel(cfg Config) (string, error) {
	if err := waitForSSH(cfg, cfg.SSHUser, 60*time.Second); err != nil {
		return "", fmt.Errorf("SSH not ready: %w", err)
	}

	upgradeCmd := "xbps-install -uy linux6.12 && xbps-reconfigure -f linux6.12"
	cmd := exec.Command("ssh", append(sshArgs(cfg), upgradeCmd)...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("kernel upgrade failed: %w", err)
	}
	scannerOut := bufio.NewScanner(stdout)
	scannerErr := bufio.NewScanner(stderr)
	go func() {
		for scannerOut.Scan() {
			addProgress("%s", scannerOut.Text())
		}
	}()
	go func() {
		for scannerErr.Scan() {
			addProgress("%s", scannerErr.Text())
		}
	}()
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("kernel upgrade failed: %w", err)
	}

	findKernel := "ls -1 /boot/vmlinux-* /boot/vmlinuz-* 2>/dev/null | sort | tail -1"
	kernelOut, err := exec.Command("ssh", append(sshArgs(cfg), findKernel)...).Output()
	if err != nil {
		return "", fmt.Errorf("failed to find kernel: %w", err)
	}
	kernelPath := strings.TrimSpace(string(kernelOut))
	if kernelPath == "" {
		return "", fmt.Errorf("no kernel found in /boot")
	}

	findInitrd := "ls -1 /boot/initramfs-*.img 2>/dev/null | sort | tail -1"
	initrdOut, err := exec.Command("ssh", append(sshArgs(cfg), findInitrd)...).Output()
	if err != nil {
		return "", fmt.Errorf("failed to find initrd: %w", err)
	}
	initrdPath := strings.TrimSpace(string(initrdOut))
	if initrdPath == "" {
		return "", fmt.Errorf("no initramfs found in /boot")
	}

	if err := copyRemoteFile(cfg, kernelPath, cfg.KernelPath); err != nil {
		return "", fmt.Errorf("failed to copy kernel: %w", err)
	}
	if err := copyRemoteFile(cfg, initrdPath, cfg.InitrdPath); err != nil {
		return "", fmt.Errorf("failed to copy initrd: %w", err)
	}

	version := filepath.Base(kernelPath)

	if err := Stop(cfg); err != nil {
		return version, fmt.Errorf("kernel updated but stop failed: %w", err)
	}
	time.Sleep(2 * time.Second)
	if err := Start(cfg); err != nil {
		return version, fmt.Errorf("kernel updated but start failed: %w", err)
	}

	return version, nil
}

func copyRemoteFile(cfg Config, remotePath, localPath string) error {
	tmpPath := localPath + ".new"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("ssh", append(sshArgs(cfg), "cat "+shellQuote(remotePath))...)
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, localPath)
}

func prepareDisk(cfg Config) (Config, error) {
	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return cfg, err
	}

	if fileExists(cfg.DiskPath) {
		if isVoidLinuxRootfsTarball(cfg.BaseImage) && !bootAssetsExist(cfg) {
			logf("Void boot assets missing; rebuilding VM disk")
			addProgress("rebuilding VM disk (boot assets missing)...")
			if err := buildVoidLinuxDisk(cfg); err != nil {
				return cfg, err
			}
		}
		return cfg, nil
	}

	addProgress("preparing VM disk for first boot...")
	addProgress("resolving base image...")
	cfg, err := resolveBaseImage(cfg)
	if err != nil {
		return cfg, err
	}

	if err := createDiskFromBaseImage(cfg); err != nil {
		return cfg, err
	}
	addProgress("VM disk ready")
	return cfg, nil
}

func resolveBaseImage(cfg Config) (Config, error) {
	if cfg.BaseImage == "" {
		cfg.BaseImage = discoverFirstFile(cfg.ImageDir, "disk")
	}
	if cfg.BaseImage == "" {
		matches, _ := filepath.Glob(filepath.Join(cfg.ImageDir, "void-aarch64-ROOTFS-*.tar.xz"))
		if len(matches) == 1 {
			cfg.BaseImage = matches[0]
		}
	}
	if cfg.BaseImage == "" {
		cfg.BaseImage = filepath.Join(cfg.ImageDir, "void-aarch64-ROOTFS.tar.xz")
	}
	if fileExists(cfg.BaseImage) {
		addProgress("base image found: %s", filepath.Base(cfg.BaseImage))
		return cfg, nil
	}
	if cfg.BaseImageURL == "" {
		url, err := resolveRootfsURL(cfg)
		if err != nil {
			return cfg, fmt.Errorf("VM_BASE_IMAGE does not exist and auto-resolve failed: %w", err)
		}
		cfg.BaseImageURL = url
	}
	expectedSize := remoteContentLength(cfg.BaseImageURL)
	if expectedSize > 0 {
		addProgress("downloading base image (%.0f MB)...", float64(expectedSize)/1024/1024)
	} else {
		addProgress("downloading base image...")
	}
	if err := ensureDownloadedFile(cfg.BaseImageURL, cfg.BaseImage); err != nil {
		return cfg, err
	}
	addProgress("base image downloaded")
	return cfg, nil
}

func createDiskFromBaseImage(cfg Config) error {
	if !fileExists(cfg.BaseImage) {
		return fmt.Errorf("VM_BASE_IMAGE does not exist: %s", cfg.BaseImage)
	}
	if isVoidLinuxRootfsTarball(cfg.BaseImage) {
		return buildVoidLinuxDisk(cfg)
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		return fmt.Errorf("missing required command: qemu-img")
	}
	if isCompressedRawImage(cfg.BaseImage) {
		return createDiskFromCompressedRaw(cfg)
	}
	return createDiskFromImageFile(cfg)
}

func createDiskFromCompressedRaw(cfg Config) error {
	logf("creating VM disk from compressed raw base image")
	addProgress("decompressing base image...")
	if err := decompressXZToRaw(cfg.BaseImage, cfg.DiskPath); err != nil {
		return err
	}
	addProgress("resizing disk to %s...", cfg.DiskSize)
	return resizeRawDisk(cfg)
}

func createDiskFromImageFile(cfg Config) error {
	format, err := diskFormat(cfg.BaseImage)
	if err != nil {
		return err
	}
	logf("creating VM disk from base image (%s)", format)
	addProgress("converting %s base image to raw disk...", format)
	if format == "raw" {
		if err := copyFile(cfg.BaseImage, cfg.DiskPath); err != nil {
			return err
		}
	} else {
		if err := runCommand("qemu-img", "convert", "-f", format, "-O", "raw", cfg.BaseImage, cfg.DiskPath); err != nil {
			return err
		}
	}
	return resizeRawDisk(cfg)
}

func resizeRawDisk(cfg Config) error {
	return runCommand("qemu-img", "resize", "-f", "raw", cfg.DiskPath, cfg.DiskSize)
}

func vfkitArgs(cfg Config) []string {
	args := []string{
		"--cpus", fmt.Sprintf("%d", cfg.CPUs),
		"--memory", fmt.Sprintf("%d", cfg.MemoryMiB),
		"--device", fmt.Sprintf("virtio-blk,path=%s", cfg.DiskPath),
		"--device", fmt.Sprintf("virtio-net,nat,mac=%s", cfg.MAC),
		"--device", "virtio-rng",
		"--device", "virtio-balloon",
		"--device", fmt.Sprintf("virtio-serial,logFilePath=%s", cfg.SerialLog),
		"--restful-uri", fmt.Sprintf("unix://%s", cfg.RestSocket),
		"--pidfile", cfg.PIDFile,
		"--log-level", "info",
	}
	if bootAssetsExist(cfg) {
		args = append(args,
			"--kernel", cfg.KernelPath,
			"--initrd", cfg.InitrdPath,
			"--kernel-cmdline", "root=/dev/vda rw console=hvc0 quiet loglevel=3",
		)
	} else {
		args = append(args, "--bootloader", fmt.Sprintf("efi,variable-store=%s,create", cfg.EFIVarsPath))
	}
	if cfg.GUI {
		args = append(args,
			"--device", "virtio-input,keyboard",
			"--device", "virtio-input,pointing",
			"--device", fmt.Sprintf("virtio-gpu,width=%d,height=%d", cfg.Width, cfg.Height),
			"--gui",
		)
	}
	return args
}

func sshArgs(cfg Config) []string {
	return sshArgsForUser(cfg, cfg.SSHUser)
}

func sshArgsForUser(cfg Config, user string) []string {
	args := []string{}
	if cfg.SSHKnownHostsFile != "" {
		args = append(args,
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "UserKnownHostsFile="+cfg.SSHKnownHostsFile,
		)
	} else {
		args = append(args,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
		)
	}
	privKey := cfg.SSHPrivateKey
	if privKey == "" {
		privKey = strings.TrimSuffix(cfg.SSHPublicKey, ".pub")
	}
	if fileExists(privKey) {
		args = append(args, "-o", "IdentitiesOnly=yes", "-i", privKey)
	}
	args = append(args, user+"@"+cfg.StaticIP)
	return args
}

func bootAssetsExist(cfg Config) bool {
	return fileExists(cfg.KernelPath) && fileExists(cfg.InitrdPath)
}

func buildVoidLinuxDisk(cfg Config) error {
	if !fileExists(cfg.SSHPublicKey) {
		return fmt.Errorf("VM_SSH_PUBLIC_KEY does not exist: %s", cfg.SSHPublicKey)
	}

	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return err
	}

	if _, err := exec.LookPath("vfkit"); err == nil {
		logf("building Void Linux VM disk using vfkit")
		addProgress("building Void Linux VM disk via vfkit (this takes several minutes)...")
		return buildVoidLinuxDiskVFKit(cfg)
	}

	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf("missing required command: vfkit or podman")
	}
	logf("building Void Linux VM disk using podman")
	addProgress("building Void Linux VM disk (this takes several minutes)...")
	builder := exec.Command(
		"podman", "run", "--rm", "--platform", "linux/arm64",
		"-e", "DISK_SIZE="+cfg.DiskSize,
		"-e", "STATIC_IP="+cfg.StaticIP,
		"-e", "CIDR="+fmt.Sprintf("%d", cfg.CIDR),
		"-e", "GATEWAY="+cfg.Gateway,
		"-e", "DNS_SERVERS="+cfg.DNSServers,
		"-e", "VM_MAC="+cfg.MAC,
		"-e", "GUEST_USER="+cfg.GuestUser,
		"-e", "GUEST_PASSWORD="+cfg.GuestPassword,
		"-e", "ROOT_PASSWORD="+cfg.RootPassword,
		"-e", "TIMEZONE="+cfg.Timezone,
		"-e", "VOID_REPOSITORY="+cfg.VoidRepository,
		"-e", "VM_NAME="+cfg.Name,
		"-e", "DEFAULT_SHELL="+cfg.DefaultShell,
		"-e", "DEFAULT_EDITOR="+cfg.DefaultEditor,
		"-e", "WINDOW_MANAGER="+cfg.WindowManager,
		"-v", cfg.ImageDir+":/repo:ro",
		"-v", cfg.StateDir+":/work",
		"-v", cfg.BaseImage+":/input/base.tar.xz:ro",
		"-v", cfg.SSHPublicKey+":/input/authorized_key.pub:ro",
		"docker.io/library/debian:stable-slim",
		"bash", "-lc", voidLinuxBuildScript(),
	)
	builder.Stdout = os.Stdout
	builder.Stderr = os.Stderr
	return builder.Run()
}

func voidLinuxBuildScript() string {
	return `
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

mkdir -p /tmp/apt-sources
cat >/tmp/apt-sources/vmctl.sources <<'EOF'
Types: deb
URIs: http://deb.debian.org/debian
Suites: stable
Components: main
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
EOF

apt-get \
  -o Dir::Etc::sourcelist=/dev/null \
  -o Dir::Etc::sourceparts=/tmp/apt-sources \
  -o Acquire::Check-Date=false \
  -o Acquire::Check-Valid-Until=false \
  update >/dev/null
apt-get \
  -o Dir::Etc::sourcelist=/dev/null \
  -o Dir::Etc::sourceparts=/tmp/apt-sources \
  -o Acquire::Check-Date=false \
  -o Acquire::Check-Valid-Until=false \
  install -y xz-utils ca-certificates e2fsprogs openssl >/dev/null

rm -f /work/disk.img /work/vmlinuz /work/initramfs.img
rm -rf /tmp/void-rootfs
mkdir -p /tmp/void-rootfs

tar -xJf /input/base.tar.xz -C /tmp/void-rootfs
cp /etc/resolv.conf /tmp/void-rootfs/etc/resolv.conf

retry_chroot_xbps() {
  local cmd="$1"
  local attempt=0
  while [ "${attempt}" -lt 8 ]; do
    attempt=$((attempt + 1))
    printf '[vmctl-build] xbps attempt %s: %s\n' "${attempt}" "${cmd}"
    if chroot /tmp/void-rootfs /bin/sh -lc "${cmd}"; then
      return 0
    fi
    sleep 10
  done
  return 1
}

repo="${VOID_REPOSITORY%/}/current"
mkdir -p /tmp/void-rootfs/etc/xbps.d
cat >/tmp/void-rootfs/etc/xbps.d/00-vmctl-repository.conf <<EOF
repository=${repo}
EOF

retry_chroot_xbps "xbps-install -R ${repo} -Sy xbps && xbps-install -R ${repo} -uy xbps"
retry_chroot_xbps "DRACUT_NO_XATTR=1 xbps-install -R ${repo} -Suy linux6.12 dracut openssh NetworkManager dbus fish-shell zsh curl wget git unzip bash file sudo chrony neovim"
retry_chroot_xbps "xbps-install -R ${repo} -Suy seatd sway foot ghostty ghostty-terminfo mesa mesa-dri wl-clipboard wofi mako grim slurp xdg-desktop-portal-wlr xorg xfce4 xfce4-terminal fcitx5 fcitx5-chinese-addons fcitx5-configtool fcitx5-gtk+2 fcitx5-gtk+3 fcitx5-gtk4 fcitx5-qt5 fcitx5-qt6 noto-fonts-cjk noto-fonts-emoji"
retry_chroot_xbps "xbps-install -R ${repo} -Suy chromium"

printf '%s\n' "${VM_NAME}" >/tmp/void-rootfs/etc/hostname

mkdir -p /tmp/void-rootfs/etc/ssh/sshd_config.d
cat >/tmp/void-rootfs/etc/ssh/sshd_config.d/99-vmctl.conf <<SSH
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
SSH

guest_shell="/bin/bash"
case "${DEFAULT_SHELL}" in
  fish) guest_shell="/usr/bin/fish" ;;
  zsh) guest_shell="/usr/bin/zsh" ;;
esac

if ! chroot /tmp/void-rootfs /usr/bin/id -u "${GUEST_USER}" >/dev/null 2>&1; then
  chroot /tmp/void-rootfs /usr/sbin/useradd -m -G wheel,audio,video,input,_seatd -s "${guest_shell}" "${GUEST_USER}"
else
  chroot /tmp/void-rootfs /usr/sbin/usermod -aG wheel,audio,video,input,_seatd "${GUEST_USER}"
  chroot /tmp/void-rootfs /usr/sbin/usermod -s "${guest_shell}" "${GUEST_USER}"
fi

if ! chroot /tmp/void-rootfs /usr/bin/getent group chrony >/dev/null 2>&1; then
  chroot /tmp/void-rootfs /usr/sbin/groupadd -r chrony
fi
if ! chroot /tmp/void-rootfs /usr/bin/id -u chrony >/dev/null 2>&1; then
  chroot /tmp/void-rootfs /usr/sbin/useradd -r -M -g chrony -s /bin/false chrony
fi

root_hash="$(openssl passwd -6 "${ROOT_PASSWORD}")"
guest_hash="$(openssl passwd -6 "${GUEST_PASSWORD}")"
chroot /tmp/void-rootfs /usr/sbin/usermod -p "${root_hash}" root
chroot /tmp/void-rootfs /usr/sbin/usermod -p "${guest_hash}" "${GUEST_USER}"

install -d -m 700 /tmp/void-rootfs/root/.ssh
install -d -m 700 /tmp/void-rootfs/home/"${GUEST_USER}"/.ssh
install -m 600 /input/authorized_key.pub /tmp/void-rootfs/root/.ssh/authorized_keys
install -m 600 /input/authorized_key.pub /tmp/void-rootfs/home/"${GUEST_USER}"/.ssh/authorized_keys

chroot /tmp/void-rootfs /usr/bin/chown -R root:root /root/.ssh
chroot /tmp/void-rootfs /usr/bin/chown -R "${GUEST_USER}:${GUEST_USER}" /home/"${GUEST_USER}"/.ssh

mkdir -p /tmp/void-rootfs/etc/sudoers.d
cat >/tmp/void-rootfs/etc/sudoers.d/10-vmctl <<SUDO
%wheel ALL=(ALL) NOPASSWD: ALL
SUDO
chmod 0440 /tmp/void-rootfs/etc/sudoers.d/10-vmctl

mkdir -p /tmp/void-rootfs/etc/NetworkManager/system-connections
cat >/tmp/void-rootfs/etc/NetworkManager/system-connections/vmctl.nmconnection <<NM
[connection]
id=vmctl
type=ethernet
autoconnect=true

[ethernet]
mac-address=${VM_MAC}

[ipv4]
method=manual
address1=${STATIC_IP}/${CIDR},${GATEWAY}
dns=${DNS_SERVERS//,/;}

[ipv6]
method=ignore
NM
chmod 0600 /tmp/void-rootfs/etc/NetworkManager/system-connections/vmctl.nmconnection

mkdir -p /tmp/void-rootfs/etc/NetworkManager/conf.d
cat >/tmp/void-rootfs/etc/NetworkManager/conf.d/10-vmctl.conf <<'EOF'
[main]
dns=none
EOF

if [ -n "${TIMEZONE:-}" ] && [ -e "/tmp/void-rootfs/usr/share/zoneinfo/${TIMEZONE}" ]; then
  ln -snf "/usr/share/zoneinfo/${TIMEZONE}" /tmp/void-rootfs/etc/localtime
  printf '%s\n' "${TIMEZONE}" >/tmp/void-rootfs/etc/timezone
fi

{
  printf '# Generated by vmctl\n'
  oldIFS="${IFS}"
  IFS=,
  for ns in ${DNS_SERVERS}; do
    printf 'nameserver %s\n' "${ns}"
  done
  IFS="${oldIFS}"
} >/tmp/void-rootfs/etc/resolv.conf

mkdir -p /tmp/void-rootfs/usr/local/bin
cat >/tmp/void-rootfs/usr/local/bin/vmctl-session <<EOF
#!/bin/sh
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export SDL_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
case "${WINDOW_MANAGER}" in
  xfce)
    export XDG_CURRENT_DESKTOP=XFCE
    export XDG_SESSION_DESKTOP=xfce
    export XDG_SESSION_TYPE=x11
    if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
      exec dbus-run-session startxfce4
    fi
    exec startxfce4
    ;;
  *)
    export XDG_CURRENT_DESKTOP=sway
    export XDG_SESSION_TYPE=wayland
    export WLR_RENDERER=pixman
    export WLR_NO_HARDWARE_CURSORS=1
    if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
      exec dbus-run-session sh -lc '
        sway &
        sway_pid=$!
        for _ in $(seq 1 100); do
          sock=$(find "${XDG_RUNTIME_DIR}" -maxdepth 1 -type s -name "wayland-*" | head -n 1)
          if [ -n "${sock}" ]; then
            export WAYLAND_DISPLAY=$(basename "${sock}")
            break
          fi
          sleep 0.1
        done
        fcitx5 -d -r >/tmp/fcitx5.log 2>&1 || true
        wait "${sway_pid}"
      '
    fi
    sway &
    sway_pid=$!
    for _ in $(seq 1 100); do
      sock=$(find "${XDG_RUNTIME_DIR}" -maxdepth 1 -type s -name "wayland-*" | head -n 1)
      if [ -n "${sock}" ]; then
        export WAYLAND_DISPLAY=$(basename "${sock}")
        break
      fi
      sleep 0.1
    done
    fcitx5 -d -r >/tmp/fcitx5.log 2>&1 || true
    wait "${sway_pid}"
    ;;
esac
EOF
chmod 0755 /tmp/void-rootfs/usr/local/bin/vmctl-session

cat >/tmp/void-rootfs/usr/local/bin/vmctl-chromium <<'EOF'
#!/bin/sh
export GTK_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
exec /usr/bin/chromium --ozone-platform=x11 "$@"
EOF
chmod 0755 /tmp/void-rootfs/usr/local/bin/vmctl-chromium

cat >/tmp/void-rootfs/usr/local/bin/vmctl-swaybar-status <<'EOF'
#!/bin/sh
printf '{"version":1}\n[\n[]\n'
while :; do
  im_name="$(fcitx5-remote -n 2>/dev/null || true)"
  case "${im_name}" in
    pinyin) im_label="中" ;;
    keyboard-us|"") im_label="EN" ;;
    *) im_label="${im_name}" ;;
  esac
  time_text="$(date '+%Y-%m-%d %H:%M:%S')"
  printf ',[{"name":"input","full_text":"IM: %s"},{"name":"time","full_text":"%s"}]\n' "${im_label}" "${time_text}"
  sleep 1
done
EOF
chmod 0755 /tmp/void-rootfs/usr/local/bin/vmctl-swaybar-status

cat >/tmp/void-rootfs/home/"${GUEST_USER}"/.bash_profile <<'EOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
EOF
chroot /tmp/void-rootfs /usr/bin/chown "${GUEST_USER}:${GUEST_USER}" /home/"${GUEST_USER}"/.bash_profile

mkdir -p /tmp/void-rootfs/home/"${GUEST_USER}"/.config/fish/conf.d
cat >/tmp/void-rootfs/home/"${GUEST_USER}"/.config/fish/conf.d/vmctl-session.fish <<'EOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    set current_tty (tty 2>/dev/null)
    if test "$current_tty" = "/dev/tty1"
      exec /usr/local/bin/vmctl-session
    end
  end
end
EOF
cat >/tmp/void-rootfs/home/"${GUEST_USER}"/.zprofile <<'EOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
EOF
chroot /tmp/void-rootfs /usr/bin/chown -R "${GUEST_USER}:${GUEST_USER}" /home/"${GUEST_USER}"/.config/fish /home/"${GUEST_USER}"/.zprofile

mkdir -p /tmp/void-rootfs/home/"${GUEST_USER}"/.config/fcitx5
mkdir -p /tmp/void-rootfs/home/"${GUEST_USER}"/.config/fcitx5/conf
cat >/tmp/void-rootfs/home/"${GUEST_USER}"/.config/fcitx5/config <<'EOF'
[Hotkey]
EnumerateWithTriggerKeys=True
EnumerateSkipFirst=False
ModifierOnlyKeyTimeout=250

[Hotkey/TriggerKeys]
0=Shift_L

[Hotkey/AltTriggerKeys]
0=Caps_Lock

[Hotkey/EnumerateForwardKeys]
0=Shift_L

[Hotkey/PrevPage]
0=Up

[Hotkey/NextPage]
0=Down

[Hotkey/PrevCandidate]
0=Shift+Tab

[Hotkey/NextCandidate]
0=Tab

[Behavior]
ActiveByDefault=False
resetStateWhenFocusIn=No
ShareInputState=No
PreeditEnabledByDefault=True
ShowInputMethodInformation=True
showInputMethodInformationWhenFocusIn=False
CompactInputMethodInformation=True
ShowFirstInputMethodInformation=True
DefaultPageSize=5
EnabledAddons=
DisabledAddons=
PreloadInputMethod=True
OverrideXkbOption=False
CustomXkbOption=
AllowInputMethodForPassword=False
ShowPreeditForPassword=False
AutoSavePeriod=30
EOF
cat >/tmp/void-rootfs/home/"${GUEST_USER}"/.config/fcitx5/profile <<'EOF'
[Groups/0]
Name=Default
Default Layout=us
DefaultIM=pinyin

[Groups/0/Items/0]
Name=keyboard-us
Layout=

[Groups/0/Items/1]
Name=pinyin
Layout=

[GroupOrder]
0=Default
EOF
chroot /tmp/void-rootfs /usr/bin/chown -R "${GUEST_USER}:${GUEST_USER}" /home/"${GUEST_USER}"/.config/fcitx5

mkdir -p /tmp/void-rootfs/home/"${GUEST_USER}"/.local/share/applications
cat >/tmp/void-rootfs/home/"${GUEST_USER}"/.local/share/applications/chromium.desktop <<'EOF'
[Desktop Entry]
Version=1.0
Name=Chromium
GenericName=Web Browser
Comment=Access the Internet
Exec=/usr/local/bin/vmctl-chromium %U
StartupNotify=true
Terminal=false
Icon=chromium
Type=Application
Categories=Network;WebBrowser;
MimeType=application/pdf;application/rdf+xml;application/rss+xml;application/xhtml+xml;application/xhtml_xml;application/xml;image/gif;image/jpeg;image/png;image/webp;text/html;text/xml;x-scheme-handler/http;x-scheme-handler/https;x-scheme-handler/chromium;
Actions=new-window;new-private-window;

[Desktop Action new-window]
Name=New Window
Exec=/usr/local/bin/vmctl-chromium

[Desktop Action new-private-window]
Name=New Incognito Window
Exec=/usr/local/bin/vmctl-chromium --incognito
EOF
chroot /tmp/void-rootfs /usr/bin/chown -R "${GUEST_USER}:${GUEST_USER}" /home/"${GUEST_USER}"/.local/share/applications

mkdir -p /tmp/void-rootfs/etc/runit/runsvdir/default
for svc in dbus sshd NetworkManager seatd chronyd; do
  if [ -d "/tmp/void-rootfs/etc/sv/${svc}" ]; then
    ln -snf "/etc/sv/${svc}" "/tmp/void-rootfs/etc/runit/runsvdir/default/${svc}"
  fi
done
cat >/tmp/void-rootfs/etc/sv/agetty-tty1/conf <<EOF
if [ -x /sbin/agetty -o -x /bin/agetty ]; then
	GETTY_ARGS="--autologin ${GUEST_USER} --noclear"
fi

BAUD_RATE=38400
TERM_NAME=linux
EOF

mkdir -p /tmp/void-rootfs/etc/sway/config.d
cat >/tmp/void-rootfs/etc/sway/config.d/10-vmctl.conf <<'EOF'
set $term ghostty
unbindsym $mod+Return
bindsym $mod+Return exec $term
set $menu wofi --show drun
unbindsym $mod+d
bindsym $mod+d exec $menu
input type:pointer {
    natural_scroll enabled
}
input type:touchpad {
    natural_scroll enabled
}
EOF

cat >/tmp/void-rootfs/etc/sway/config.d/20-vmctl-bar.conf <<'EOF'
bar bar-0 {
    tray_output *
    status_command /usr/local/bin/vmctl-swaybar-status
}
EOF

chroot /tmp/void-rootfs /bin/sh -lc "DRACUT_NO_XATTR=1 xbps-reconfigure -fa || true"

kernel="$(
  find /tmp/void-rootfs/boot -maxdepth 1 -type f \( -name 'vmlinux-*' -o -name 'Image*' -o -name 'vmlinuz-*' \) \
    | sort | tail -n 1
)"
initrd="$(find /tmp/void-rootfs/boot -maxdepth 1 -type f -name 'initramfs-*.img' | sort | tail -n 1)"
if [ -z "${kernel}" ] || [ -z "${initrd}" ]; then
  printf 'missing boot assets after Void provisioning\n' >&2
  find /tmp/void-rootfs/boot -maxdepth 2 -type f | sort >&2 || true
  exit 1
fi

cp "${kernel}" /work/vmlinuz
cp "${initrd}" /work/initramfs.img

truncate -s "${DISK_SIZE}" /work/disk.img
mkfs.ext4 -F -L rootfs -d /tmp/void-rootfs /work/disk.img
`
}

func ClipboardIn(cfg Config) error {
	if _, err := exec.LookPath("pbpaste"); err != nil {
		return fmt.Errorf("missing required command: pbpaste")
	}
	ssh := exec.Command("ssh", append(sshArgsForUser(cfg, cfg.GuestUser), waylandClipboardShell("wl-copy"))...)
	pbpaste := exec.Command("pbpaste")

	reader, writer := io.Pipe()
	pbpaste.Stdout = writer
	ssh.Stdin = reader
	ssh.Stdout = os.Stdout
	ssh.Stderr = os.Stderr
	pbpaste.Stderr = os.Stderr

	if err := ssh.Start(); err != nil {
		return err
	}
	if err := pbpaste.Start(); err != nil {
		_ = ssh.Process.Kill()
		return err
	}

	pbErr := pbpaste.Wait()
	_ = writer.Close()
	sshErr := ssh.Wait()
	_ = reader.Close()
	if pbErr != nil {
		return pbErr
	}
	return sshErr
}

func ClipboardOut(cfg Config) error {
	if _, err := exec.LookPath("pbcopy"); err != nil {
		return fmt.Errorf("missing required command: pbcopy")
	}
	ssh := exec.Command("ssh", append(sshArgsForUser(cfg, cfg.GuestUser), waylandClipboardShell("wl-paste --no-newline"))...)
	pbcopy := exec.Command("pbcopy")

	reader, writer := io.Pipe()
	ssh.Stdout = writer
	ssh.Stderr = os.Stderr
	pbcopy.Stdin = reader
	pbcopy.Stdout = os.Stdout
	pbcopy.Stderr = os.Stderr

	if err := pbcopy.Start(); err != nil {
		return err
	}
	if err := ssh.Start(); err != nil {
		_ = pbcopy.Process.Kill()
		return err
	}

	sshErr := ssh.Wait()
	_ = writer.Close()
	pbErr := pbcopy.Wait()
	_ = reader.Close()
	if sshErr != nil {
		return sshErr
	}
	return pbErr
}

func waylandClipboardShell(command string) string {
	return "sh -lc " + shellQuote(`uid="$(id -u)"; runtime_dir="${HOME}/.local/run"; [ -d "${runtime_dir}" ] || runtime_dir="/run/user/${uid}"; export XDG_RUNTIME_DIR="${runtime_dir}"; sock="$(find "${XDG_RUNTIME_DIR}" -maxdepth 1 -type s -name 'wayland-*' | head -n 1)"; [ -n "${sock}" ] || { echo "no Wayland socket found; log into Sway first" >&2; exit 1; }; export WAYLAND_DISPLAY="$(basename "${sock}")"; `+command)
}

func fixGuestConfig(cfg Config) error {
	pubKey, err := os.ReadFile(cfg.SSHPublicKey)
	if err != nil {
		return err
	}
	key := shellQuote(strings.TrimSpace(string(pubKey)))
	guest := shellQuote(cfg.GuestUser)
	script := fmt.Sprintf(`set -e
guest=%s
pubkey=%s

mkdir -p /home/"${guest}"/.ssh /home/"${guest}"/.config/fish/conf.d
printf '%%s\n' "${pubkey}" > /home/"${guest}"/.ssh/authorized_keys
chmod 700 /home/"${guest}"/.ssh
chmod 600 /home/"${guest}"/.ssh/authorized_keys

cat > /home/"${guest}"/.bash_profile <<'EOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
EOF

cat > /home/"${guest}"/.zprofile <<'EOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
EOF

cat > /home/"${guest}"/.config/fish/conf.d/vmctl-session.fish <<'EOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    if string match -q /dev/tty1 (tty 2>/dev/null)
      exec /usr/local/bin/vmctl-session
    end
  end
end
EOF

chown -R "${guest}:${guest}" /home/"${guest}" 2>/dev/null || true
passwd -d "${guest}" >/dev/null 2>&1 || true
usermod -U "${guest}" >/dev/null 2>&1 || true
grep -q '^PerSourcePenalties no$' /etc/ssh/sshd_config.d/99-vmctl.conf || printf '\nPerSourcePenalties no\n' >> /etc/ssh/sshd_config.d/99-vmctl.conf
sv restart sshd >/dev/null 2>&1 || true

test -s /home/"${guest}"/.ssh/authorized_keys
test -s /home/"${guest}"/.bash_profile
test -s /home/"${guest}"/.config/fish/conf.d/vmctl-session.fish
echo DONE
`, guest, key)

	cmd := exec.Command("ssh", append(sshArgsForUser(cfg, "root"), "sh -s")...)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fixGuestConfig: %w\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "DONE") {
		return fmt.Errorf("fixGuestConfig incomplete:\n%s", string(out))
	}
	logf("guest configuration repaired")
	return nil
}

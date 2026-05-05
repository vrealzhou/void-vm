package vmctl

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"text/template"
	"time"
)

func resolveBuildKernelURL(cfg Config) (string, error) {
	if cfg.BuildKernelURL != "" {
		return cfg.BuildKernelURL, nil
	}

	cacheFile := filepath.Join(cfg.StateDir, "build-kernel-url.cache")
	if data, err := os.ReadFile(cacheFile); err == nil {
		cached := strings.TrimSpace(string(data))
		if cached != "" {
			return cached, nil
		}
	}

	repoBase := strings.TrimRight(cfg.VoidRepository, "/") + "/current/aarch64/"
	addProgress("resolving latest kernel package from %s ...", repoBase)

	client := downloadHTTPClient(60 * time.Second)
	resp, err := client.Get(repoBase)
	if err != nil {
		return "", fmt.Errorf("failed to fetch repository index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("repository index returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read repository index: %w", err)
	}

	re := regexp.MustCompile(`linux6\.12-[0-9][^"']*\.aarch64\.xbps`)
	allMatches := re.FindAllString(string(body), -1)
	if len(allMatches) == 0 {
		return "", fmt.Errorf("no linux6.12 kernel package found in repository index")
	}

	pkg := allMatches[len(allMatches)-1]
	url := repoBase + pkg
	addProgress("resolved kernel package: %s", pkg)

	if err := os.MkdirAll(cfg.StateDir, 0o755); err == nil {
		_ = os.WriteFile(cacheFile, []byte(url+"\n"), 0o644)
	}

	return url, nil
}

func resolveRootfsURL(cfg Config) (string, error) {
	cacheFile := filepath.Join(cfg.StateDir, "rootfs-url.cache")
	if data, err := os.ReadFile(cacheFile); err == nil {
		cached := strings.TrimSpace(string(data))
		if cached != "" {
			return cached, nil
		}
	}

	repoBase := strings.TrimRight(cfg.VoidRepository, "/") + "/live/current/"
	addProgress("resolving latest rootfs tarball from %s ...", repoBase)

	client := downloadHTTPClient(60 * time.Second)
	resp, err := client.Get(repoBase)
	if err != nil {
		return "", fmt.Errorf("failed to fetch live index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("live index returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read live index: %w", err)
	}

	re := regexp.MustCompile(`void-aarch64-ROOTFS-[^"']+\.tar\.xz`)
	matches := re.FindAllString(string(body), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no void-aarch64-ROOTFS tarball found in live index")
	}

	pkg := matches[len(matches)-1]
	url := repoBase + pkg
	addProgress("resolved rootfs tarball: %s", pkg)

	if err := os.MkdirAll(cfg.StateDir, 0o755); err == nil {
		_ = os.WriteFile(cacheFile, []byte(url+"\n"), 0o644)
	}

	return url, nil
}

func downloadBuildKernel(cfg Config) (string, error) {
	buildKernel := filepath.Join(cfg.StateDir, "build-vmlinuz")
	buildModules := filepath.Join(cfg.StateDir, "build-kernel-modules")
	if fileExists(buildKernel) && fileExists(buildModules) {
		return buildKernel, nil
	}

	kernelURL, err := resolveBuildKernelURL(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to resolve kernel URL: %w", err)
	}

	kernelXbps := filepath.Join(cfg.StateDir, "linux-build.xbps")
	if !fileExists(kernelXbps) {
		addProgress("downloading kernel package for build VM...")
		if err := ensureDownloadedFile(kernelURL, kernelXbps); err != nil {
			return "", fmt.Errorf("failed to download kernel package: %w", err)
		}
	}

	addProgress("extracting kernel from xbps package...")
	tmpDir, err := os.MkdirTemp("", "vmctl-xbps-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("tar", "-xf", kernelXbps, "-C", tmpDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to extract kernel package: %w", err)
	}

	matches, _ := filepath.Glob(filepath.Join(tmpDir, "boot", "vmlinuz-*"))
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(tmpDir, "boot", "vmlinux-*"))
	}
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(tmpDir, "boot", "Image-*"))
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no kernel found in xbps package under boot/")
	}

	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return "", err
	}
	if err := copyFile(matches[len(matches)-1], buildKernel); err != nil {
		return "", err
	}
	addProgress("build kernel extracted: %s", filepath.Base(matches[len(matches)-1]))

	if !fileExists(buildModules) {
		modulePatterns := []string{
			"usr/lib/modules/*/kernel/drivers/virtio/*",
			"usr/lib/modules/*/kernel/drivers/block/virtio_blk.ko.zst",
			"usr/lib/modules/*/kernel/drivers/net/virtio_net.ko.zst",
			"usr/lib/modules/*/kernel/drivers/net/net_failover.ko.zst",
			"usr/lib/modules/*/kernel/net/core/failover.ko.zst",
			"usr/lib/modules/*/kernel/drivers/char/virtio_console.ko.zst",
			"usr/lib/modules/*/modules.*",
			"usr/lib/modules/*/build",
			"usr/lib/modules/*/source",
		}
		addProgress("extracting kernel modules for build VM...")
		os.RemoveAll(buildModules)
		for _, pat := range modulePatterns {
			fullPat := filepath.Join(tmpDir, pat)
			pmatches, _ := filepath.Glob(fullPat)
			for _, pm := range pmatches {
				rel, _ := filepath.Rel(tmpDir, pm)
				dst := filepath.Join(buildModules, rel)
				os.MkdirAll(filepath.Dir(dst), 0o755)
				copyFile(pm, dst)
			}
		}
	}

	return buildKernel, nil
}

func prepareBuildRootfs(cfg Config, rootfsDir string) error {
	addProgress("extracting Void rootfs tarball...")
	cmd := exec.Command("tar", "-xJf", cfg.BaseImage, "-C", rootfsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract rootfs: %w", err)
	}

	addProgress("injecting build configuration into rootfs...")

	buildModules := filepath.Join(cfg.StateDir, "build-kernel-modules")
	if fileExists(buildModules) {
		addProgress("injecting kernel modules into rootfs...")
		filepath.WalkDir(buildModules, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(buildModules, path)
			dst := filepath.Join(rootfsDir, rel)
			os.MkdirAll(filepath.Dir(dst), 0o755)
			copyFile(path, dst)
			return nil
		})
	}

	repo := strings.TrimRight(cfg.VoidRepository, "/") + "/current/aarch64"
	xbpsDir := filepath.Join(rootfsDir, "etc", "xbps.d")
	if err := os.MkdirAll(xbpsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(xbpsDir, "00-vmctl-repository.conf"), []byte("repository="+repo+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(xbpsDir, "00-ssl.conf"), []byte("ssl_verify=no\n"), 0o644); err != nil {
		return err
	}

	sshDir := filepath.Join(rootfsDir, "root", ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	pubKey, err := os.ReadFile(cfg.SSHPublicKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sshDir, "authorized_keys"), pubKey, 0o600); err != nil {
		return err
	}

	sshdDir := filepath.Join(rootfsDir, "etc", "ssh", "sshd_config.d")
	if err := os.MkdirAll(sshdDir, 0o755); err != nil {
		return err
	}
	sshdConf := "PermitRootLogin prohibit-password\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nStrictModes no\nPerSourcePenalties no\nMaxStartups 100:100:100\nLogLevel DEBUG3\n"
	if err := os.WriteFile(filepath.Join(sshdDir, "99-vmctl.conf"), []byte(sshdConf), 0o644); err != nil {
		return err
	}

	resolvConf := ""
	for _, ns := range strings.Split(cfg.DNSServers, ",") {
		resolvConf += "nameserver " + strings.TrimSpace(ns) + "\n"
	}
	if err := os.WriteFile(filepath.Join(rootfsDir, "etc", "resolv.conf"), []byte(resolvConf), 0o644); err != nil {
		return err
	}

	for _, svc := range []string{"sshd", "dbus"} {
		src := filepath.Join(rootfsDir, "etc", "sv", svc)
		dst := filepath.Join(rootfsDir, "etc", "runit", "runsvdir", "default", svc)
		if fileExists(src) && !fileExists(dst) {
			_ = os.Symlink("/etc/sv/"+svc, dst)
		}
	}

	if err := os.WriteFile(filepath.Join(rootfsDir, "etc", "hostname"), []byte(cfg.Name+"\n"), 0o644); err != nil {
		return err
	}

	initScript := `#!/bin/sh
export PATH=/usr/bin:/usr/sbin:/bin:/sbin

mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mkdir -p /dev/pts /run /tmp
mount -t devpts devpts /dev/pts
mount -t tmpfs tmpfs /run

ip link set lo up

echo "=== loading virtio modules ===" > /dev/hvc0
kver="$(ls /usr/lib/modules/ | head -1)"
if [ -n "$kver" ]; then
	depmod -a "$kver" 2>/dev/null
	modprobe virtio_net 2>/dev/null
	modprobe virtio_blk 2>/dev/null
	modprobe virtio_console 2>/dev/null
fi

echo "=== waiting for network interface ===" > /dev/hvc0

iface=""
for i in $(seq 1 60); do
	for dev in /sys/class/net/*; do
		name="$(basename "$dev")"
		case "$name" in
			lo) continue ;;
		esac
		iface="$name"
		break 2
	done
	sleep 1
done

echo "found interface: ${iface:-NONE}" > /dev/hvc0

if [ -n "$iface" ]; then
	ip link set "$iface" up
	echo "configuring static 192.168.64.99 on ${iface}" > /dev/hvc0
	ip addr add 192.168.64.99/24 dev "$iface" 2>/dev/null || true
	ip route add default via 192.168.64.1 2>/dev/null || true
fi

echo "NETWORK_READY" > /dev/hvc0
ip addr > /dev/hvc0 2>&1
ip route > /dev/hvc0 2>&1

mkdir -p /etc/ssh /run/sshd
ssh-keygen -A 2>/dev/null
chmod 700 /root /root/.ssh
chmod 600 /root/.ssh/authorized_keys
rm -f /etc/nologin /run/nologin /var/run/nologin
mkdir -p /var/chroot/ssh
chown root:root /var/chroot /var/chroot/ssh 2>/dev/null || true
chmod 755 /var/chroot /var/chroot/ssh 2>/dev/null || true
chmod 755 /run/sshd 2>/dev/null || true
/usr/bin/sshd -D -e -f /etc/ssh/sshd_config >/dev/hvc0 2>&1 &

echo "BUILD_VM_READY" > /dev/hvc0

# Keep PID 1 simple for the build VM. Running the normal system init here
# causes a second rootfs boot sequence, which re-mounts / and drops into an
# emergency shell before the host can complete the SSH-driven build.
while :; do
	sleep 300
done
`
	if err := os.WriteFile(filepath.Join(rootfsDir, "init"), []byte(initScript), 0o755); err != nil {
		return err
	}

	return nil
}

func createCpioInitrd(rootfsDir, outputPath string) error {
	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	findCmd := exec.Command("find", ".")
	findCmd.Dir = rootfsDir

	cpioCmd := exec.Command("cpio", "-o", "-H", "newc")
	cpioCmd.Dir = rootfsDir
	cpioCmd.Stderr = os.Stderr

	gzipCmd := exec.Command("gzip", "-1")
	gzipCmd.Stderr = os.Stderr

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		return err
	}

	findCmd.Stdout = pipeW
	cpioCmd.Stdin = pipeR

	gzipR, gzipW, err := os.Pipe()
	if err != nil {
		pipeR.Close()
		pipeW.Close()
		return err
	}

	cpioCmd.Stdout = gzipW
	gzipCmd.Stdin = gzipR
	gzipCmd.Stdout = f

	if err := findCmd.Start(); err != nil {
		return err
	}
	pipeW.Close()

	if err := cpioCmd.Start(); err != nil {
		return err
	}
	pipeR.Close()

	if err := gzipCmd.Start(); err != nil {
		return err
	}
	gzipW.Close()

	if err := findCmd.Wait(); err != nil {
		return err
	}
	if err := cpioCmd.Wait(); err != nil {
		return err
	}
	return gzipCmd.Wait()
}

func dirSizeMB(dir string) float64 {
	var size int64
	filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return float64(size) / 1024 / 1024
}

type buildVMScriptData struct {
	Repo          string
	RealRepo      string
	Name          string
	DefaultShell  string
	GuestUser     string
	RootPassword  string
	GuestPassword string
	MAC           string
	StaticIP      string
	CIDR          int
	Gateway       string
	DNS           string
	Timezone      string
	SSHPublicKey  string
}

func buildVMScript(cfg Config, realVoidRepo string) string {
	data := buildVMScriptData{
		Repo:          strings.TrimRight(cfg.VoidRepository, "/") + "/current/aarch64",
		RealRepo:      strings.TrimRight(realVoidRepo, "/") + "/current/aarch64",
		Name:          cfg.Name,
		DefaultShell:  cfg.DefaultShell,
		GuestUser:     cfg.GuestUser,
		RootPassword:  cfg.RootPassword,
		GuestPassword: cfg.GuestPassword,
		MAC:           cfg.MAC,
		StaticIP:      cfg.StaticIP,
		CIDR:          cfg.CIDR,
		Gateway:       cfg.Gateway,
		DNS:           strings.ReplaceAll(cfg.DNSServers, ",", ";"),
		Timezone:      cfg.Timezone,
	}
	pubKey, _ := os.ReadFile(cfg.SSHPublicKey)
	if pubKey != nil {
		data.SSHPublicKey = strings.TrimSpace(string(pubKey))
	}

	const script = `#!/bin/sh
set -euo pipefail

# Point xbps at the proxy
mkdir -p /etc/xbps.d
echo "repository={{.Repo}}" > /etc/xbps.d/00-vmctl-repository.conf
echo "ssl_verify=no" > /etc/xbps.d/00-ssl.conf

REPO="{{.Repo}}"
TARGET="/mnt/target"

retry_xbps() {
  local cmd="$1"
  local attempt=0
  while [ "${attempt}" -lt 8 ]; do
    attempt=$((attempt + 1))
    printf '[vmctl-build] xbps attempt %s: %s\n' "${attempt}" "${cmd}"
    if sh -lc "${cmd}" </dev/null; then
      return 0
    fi
    sleep 10
  done
  return 1
}

retry_xbps "xbps-install -y -R ${REPO} -Sy xbps && xbps-install -y -R ${REPO} -uy xbps shadow"

mkfs.ext4 -F -L rootfs /dev/vda
mkdir -p ${TARGET}
mount /dev/vda ${TARGET}

mkdir -p ${TARGET}/etc/xbps.d
echo "repository=${REPO}" > ${TARGET}/etc/xbps.d/00-vmctl-repository.conf
echo "ssl_verify=no" > ${TARGET}/etc/xbps.d/00-ssl.conf
mkdir -p ${TARGET}/var/db/xbps/keys
if [ -d /var/db/xbps/keys ]; then
  cp -a /var/db/xbps/keys/. ${TARGET}/var/db/xbps/keys/
fi
if [ -d /usr/share/xbps.d/keys ]; then
  mkdir -p ${TARGET}/usr/share/xbps.d/keys
  cp -a /usr/share/xbps.d/keys/. ${TARGET}/usr/share/xbps.d/keys/
fi

retry_xbps "DRACUT_NO_XATTR=1 xbps-install -y -R ${REPO} -Suy --root=${TARGET} base-system linux6.12 dracut openssh NetworkManager dbus fish-shell zsh curl wget git unzip bash file sudo chrony neovim helix"

retry_xbps "xbps-install -y -R ${REPO} -Suy --root=${TARGET} seatd sway foot ghostty ghostty-terminfo mesa mesa-dri wl-clipboard wofi mako grim slurp xdg-desktop-portal-wlr xorg xfce4 xfce4-terminal fcitx5 fcitx5-chinese-addons fcitx5-configtool fcitx5-gtk+2 fcitx5-gtk+3 fcitx5-gtk4 fcitx5-qt5 fcitx5-qt6 noto-fonts-cjk noto-fonts-emoji font-sarasa-gothic"

retry_xbps "xbps-install -y -R ${REPO} -Suy --root=${TARGET} chromium"

printf '%s\n' "{{.Name}}" > ${TARGET}/etc/hostname
echo "{{.Gateway}} host.vm" >> ${TARGET}/etc/hosts

mkdir -p ${TARGET}/etc/ssh/sshd_config.d
cat > ${TARGET}/etc/ssh/sshd_config.d/99-vmctl.conf <<SSH
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
PerSourcePenalties no
MaxStartups 100:30:100
MaxSessions 20
SSH

guest_shell="/bin/bash"
case "{{.DefaultShell}}" in
  fish) guest_shell="/usr/bin/fish" ;;
  zsh) guest_shell="/usr/bin/zsh" ;;
esac

install -d -m 700 ${TARGET}/root/.ssh
printf '%s\n' "{{.SSHPublicKey}}" > ${TARGET}/root/.ssh/authorized_keys
chmod 600 ${TARGET}/root/.ssh/authorized_keys

mkdir -p ${TARGET}/etc/sudoers.d
cat > ${TARGET}/etc/sudoers.d/10-vmctl <<SUDO
%wheel ALL=(ALL) NOPASSWD: ALL
SUDO
chmod 0440 ${TARGET}/etc/sudoers.d/10-vmctl

mkdir -p ${TARGET}/etc/NetworkManager/system-connections
cat > ${TARGET}/etc/NetworkManager/system-connections/vmctl.nmconnection <<NM
[connection]
id=vmctl
type=ethernet
autoconnect=true

[ethernet]
mac-address={{.MAC}}

[ipv4]
method=manual
address1={{.StaticIP}}/{{.CIDR}},{{.Gateway}}
dns={{.DNS}}

[ipv6]
method=ignore
NM
chmod 0600 ${TARGET}/etc/NetworkManager/system-connections/vmctl.nmconnection

mkdir -p ${TARGET}/etc/NetworkManager/conf.d
cat > ${TARGET}/etc/NetworkManager/conf.d/10-vmctl.conf <<'EOF'
[main]
dns=none
EOF

if [ -n "{{.Timezone}}" ] && [ -e "${TARGET}/usr/share/zoneinfo/{{.Timezone}}" ]; then
  ln -snf "/usr/share/zoneinfo/{{.Timezone}}" ${TARGET}/etc/localtime
  printf '%s\n' "{{.Timezone}}" > ${TARGET}/etc/timezone
fi

cp /etc/resolv.conf ${TARGET}/etc/resolv.conf

mkdir -p ${TARGET}/usr/local/bin
cat > ${TARGET}/usr/local/bin/vmctl-session <<'SESSIONEOF'
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
SESSIONEOF
chmod 0755 ${TARGET}/usr/local/bin/vmctl-session

cat > ${TARGET}/usr/local/bin/vmctl-chromium <<'CHROMEOF'
#!/bin/sh
export GTK_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
exec /usr/bin/chromium --ozone-platform=x11 "$@"
CHROMEOF
chmod 0755 ${TARGET}/usr/local/bin/vmctl-chromium

cat > ${TARGET}/usr/local/bin/vmctl-swaybar-status <<'BARSTATUSEOF'
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
BARSTATUSEOF
chmod 0755 ${TARGET}/usr/local/bin/vmctl-swaybar-status

mkdir -p ${TARGET}/etc/runit/runsvdir/default
for svc in dbus sshd NetworkManager seatd chronyd; do
  if [ -d "${TARGET}/etc/sv/${svc}" ]; then
    ln -snf "/etc/sv/${svc}" "${TARGET}/etc/runit/runsvdir/default/${svc}"
  fi
done
mkdir -p ${TARGET}/etc/sv/agetty-tty1
cat > ${TARGET}/etc/sv/agetty-tty1/conf <<AGETTYCONF
if [ -x /sbin/agetty -o -x /bin/agetty ]; then
	GETTY_ARGS="--autologin {{.GuestUser}} --noclear"
fi

BAUD_RATE=38400
TERM_NAME=linux
AGETTYCONF

mkdir -p ${TARGET}/etc/sway/config.d
cat > ${TARGET}/etc/sway/config.d/10-vmctl.conf <<'SWAYCONF'
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
SWAYCONF

cat > ${TARGET}/etc/sway/config.d/20-vmctl-bar.conf <<'SWAYBARCONF'
bar bar-0 {
    tray_output *
    status_command /usr/local/bin/vmctl-swaybar-status
}
SWAYBARCONF

DRACUT_NO_XATTR=1 xbps-reconfigure -r ${TARGET} -fa || true

echo "repository={{.RealRepo}}" > ${TARGET}/etc/xbps.d/00-vmctl-repository.conf

set +euo pipefail

if ! grep -q '^{{.GuestUser}}:' ${TARGET}/etc/passwd; then
  useradd -R ${TARGET} -m -s "${guest_shell}" "{{.GuestUser}}" 2>/dev/null
  for grp in wheel audio video input _seatd; do
    grep -q "^${grp}:" ${TARGET}/etc/group && usermod -R ${TARGET} -aG "${grp}" "{{.GuestUser}}" 2>/dev/null
  done
else
  usermod -R ${TARGET} -aG wheel,audio,video,input,_seatd "{{.GuestUser}}" 2>/dev/null
  usermod -R ${TARGET} -s "${guest_shell}" "{{.GuestUser}}" 2>/dev/null
fi

if ! grep -q '^chrony:' ${TARGET}/etc/group; then
  groupadd -R ${TARGET} -r chrony 2>/dev/null
fi
if ! grep -q '^chrony:' ${TARGET}/etc/passwd; then
  useradd -R ${TARGET} -r -M -g chrony -s /bin/false chrony 2>/dev/null
fi

printf '%s\n%s\n' "root:{{.RootPassword}}" "{{.GuestUser}}:{{.GuestPassword}}" | chpasswd -R ${TARGET} || true
guest_ids="$(awk -F: '$1=="{{.GuestUser}}" {print $3 ":" $4}' ${TARGET}/etc/passwd)"
if [ -z "${guest_ids}" ]; then
  echo "failed to resolve uid/gid for {{.GuestUser}}" >&2
  exit 1
fi
chown -R 0:0 ${TARGET}/root/.ssh
chown -R "${guest_ids}" ${TARGET}/home/"{{.GuestUser}}"
mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.local/run
chown "${guest_ids}" ${TARGET}/home/"{{.GuestUser}}"/.local/run
chmod 700 ${TARGET}/home/"{{.GuestUser}}"/.local/run

install -d -m 700 ${TARGET}/home/"{{.GuestUser}}"/.ssh
printf '%s\n' "{{.SSHPublicKey}}" > ${TARGET}/home/"{{.GuestUser}}"/.ssh/authorized_keys
chmod 600 ${TARGET}/home/"{{.GuestUser}}"/.ssh/authorized_keys

cat > ${TARGET}/home/"{{.GuestUser}}"/.bash_profile <<'BASHPROF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
BASHPROF

mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.config/fish/conf.d
cat > ${TARGET}/home/"{{.GuestUser}}"/.config/fish/conf.d/vmctl-session.fish <<'FISHEOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    if string match -q /dev/tty1 (tty 2>/dev/null)
      exec /usr/local/bin/vmctl-session
    end
  end
end
FISHEOF
cat > ${TARGET}/home/"{{.GuestUser}}"/.zprofile <<'ZPROFILEOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
ZPROFILEOF

mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.config/fcitx5
mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.config/fcitx5/conf
cat > ${TARGET}/home/"{{.GuestUser}}"/.config/fcitx5/config <<'FCITX5CONFIG'
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
FCITX5CONFIG
cat > ${TARGET}/home/"{{.GuestUser}}"/.config/fcitx5/profile <<'FCITX5PROFILE'
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
FCITX5PROFILE

mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.local/share/applications
cat > ${TARGET}/home/"{{.GuestUser}}"/.local/share/applications/chromium.desktop <<'CHROMIUMDESKTOP'
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
CHROMIUMDESKTOP

chown -R "${guest_ids}" ${TARGET}/home/"{{.GuestUser}}"

set -euo pipefail

mkdir -p ${TARGET}/etc/fonts/conf.d
cat > ${TARGET}/etc/fonts/conf.d/50-vmctl-cjk.conf <<'FONTCONF'
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>
  <match target="font">
    <test name="family" compare="contains">
      <string>Sarasa</string>
    </test>
    <edit name="hintstyle" mode="assign"><const>hintslight</const></edit>
    <edit name="antialias" mode="assign"><bool>true</bool></edit>
  </match>
  <alias>
    <family>sans-serif</family>
    <prefer>
      <family>Sarasa Gothic SC</family>
      <family>Noto Sans CJK SC</family>
    </prefer>
  </alias>
  <alias>
    <family>serif</family>
    <prefer>
      <family>Noto Serif CJK SC</family>
    </prefer>
  </alias>
  <alias>
    <family>monospace</family>
    <prefer>
      <family>Sarasa Mono SC</family>
      <family>Noto Sans Mono CJK SC</family>
    </prefer>
  </alias>
</fontconfig>
FONTCONF

set +e
mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.ssh 2>/dev/null
printf '%s\n' "{{.SSHPublicKey}}" > ${TARGET}/home/"{{.GuestUser}}"/.ssh/authorized_keys 2>/dev/null
chmod 700 ${TARGET}/home/"{{.GuestUser}}"/.ssh 2>/dev/null
chmod 600 ${TARGET}/home/"{{.GuestUser}}"/.ssh/authorized_keys 2>/dev/null
mkdir -p ${TARGET}/home/"{{.GuestUser}}"/.config/fish/conf.d 2>/dev/null
cat > ${TARGET}/home/"{{.GuestUser}}"/.config/fish/conf.d/vmctl-session.fish << 'FEOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    if string match -q /dev/tty1 (tty 2>/dev/null)
      exec /usr/local/bin/vmctl-session
    end
  end
end
FEOF
chown -R "{{.GuestUser}}:{{.GuestUser}}" ${TARGET}/home/"{{.GuestUser}}" 2>/dev/null || true
set -e

test -s ${TARGET}/root/.ssh/authorized_keys
test -s ${TARGET}/home/"{{.GuestUser}}"/.ssh/authorized_keys
test -s ${TARGET}/home/"{{.GuestUser}}"/.bash_profile
test -s ${TARGET}/home/"{{.GuestUser}}"/.config/fish/conf.d/vmctl-session.fish
grep -q '^{{.GuestUser}}:' ${TARGET}/etc/passwd
grep -q '^{{.GuestUser}}:[^!*]' ${TARGET}/etc/shadow

kernel="$(
  find ${TARGET}/boot -maxdepth 1 -type f \( -name 'vmlinux-*' -o -name 'Image*' -o -name 'vmlinuz-*' \) \
    | sort | tail -n 1
)"
initrd="$(find ${TARGET}/boot -maxdepth 1 -type f -name 'initramfs-*.img' | sort | tail -n 1)"
if [ -z "${kernel}" ] || [ -z "${initrd}" ]; then
  printf 'missing boot assets after Void provisioning\n' >&2
  find ${TARGET}/boot -maxdepth 2 -type f | sort >&2 || true
  exit 1
fi

echo "KERNEL=${kernel}"
echo "INITRD=${initrd}"
echo "BUILD_SUCCESS"
`

	t := template.Must(template.New("build").Parse(script))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String()
}

func buildVoidLinuxDiskVFKit(cfg Config) error {
	addProgress("downloading build kernel...")
	buildKernel, err := downloadBuildKernel(cfg)
	if err != nil {
		return fmt.Errorf("failed to download build kernel: %w", err)
	}

	addProgress("preparing build rootfs...")
	rootfsDir, err := os.MkdirTemp("", "vmctl-rootfs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootfsDir)

	if err := prepareBuildRootfs(cfg, rootfsDir); err != nil {
		return fmt.Errorf("failed to prepare build rootfs: %w", err)
	}

	rootfsSize := dirSizeMB(rootfsDir)
	addProgress("rootfs size: %.0f MB", rootfsSize)

	buildInitrd := filepath.Join(cfg.StateDir, "build-initrd.cpio.gz")
	addProgress("creating cpio initrd...")
	if err := createCpioInitrd(rootfsDir, buildInitrd); err != nil {
		return fmt.Errorf("failed to create initrd: %w", err)
	}
	os.RemoveAll(rootfsDir)

	buildCfg := cfg
	buildCfg.Name = cfg.Name + "-build"
	buildCfg.PIDFile = filepath.Join(cfg.StateDir, "build-vfkit.pid")
	buildCfg.RestSocket = filepath.Join(cfg.StateDir, "build-vfkit.sock")
	buildCfg.LogFile = filepath.Join(cfg.StateDir, "build-vfkit.log")
	buildCfg.SerialLog = filepath.Join(cfg.StateDir, "build-serial.log")
	buildScriptLog := filepath.Join(cfg.StateDir, "build-script.log")
	buildCfg.MAC = "52:54:00:64:00:BB"
	buildCfg.StaticIP = "192.168.64.99"
	buildCfg.CPUs = 4
	buildCfg.MemoryMiB = 4096
	buildCfg.SSHUser = "root"
	buildCfg.SSHKnownHostsFile = ""

	cleanup := true
	defer func() {
		addProgress("cleaning up build VM...")
		Stop(buildCfg)
		os.Remove(buildCfg.PIDFile)
		os.Remove(buildCfg.RestSocket)
		if cleanup {
			os.Remove(buildCfg.LogFile)
			os.Remove(buildCfg.SerialLog)
			os.Remove(buildScriptLog)
		}
		os.Remove(buildInitrd)
	}()

	targetDisk := cfg.DiskPath + ".building"
	os.Remove(targetDisk)
	addProgress("creating target disk (%s)...", cfg.DiskSize)
	if err := createSparseFile(targetDisk, cfg.DiskSize); err != nil {
		return fmt.Errorf("failed to create target disk: %w", err)
	}

	_ = os.Remove(buildCfg.RestSocket)

	args := []string{
		"--cpus", fmt.Sprintf("%d", buildCfg.CPUs),
		"--memory", fmt.Sprintf("%d", buildCfg.MemoryMiB),
		"--device", fmt.Sprintf("virtio-blk,path=%s", targetDisk),
		"--device", fmt.Sprintf("virtio-net,nat,mac=%s", buildCfg.MAC),
		"--device", "virtio-rng",
		"--device", fmt.Sprintf("virtio-serial,logFilePath=%s", buildCfg.SerialLog),
		"--restful-uri", fmt.Sprintf("unix://%s", buildCfg.RestSocket),
		"--pidfile", buildCfg.PIDFile,
		"--kernel", buildKernel,
		"--initrd", buildInitrd,
		"--kernel-cmdline", "console=hvc0 quiet",
		"--log-level", "info",
	}

	logFile, err := os.OpenFile(buildCfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	addProgress("launching build VM...")
	cmd := exec.Command("vfkit", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start build VM: %w", err)
	}
	_ = cmd.Process.Release()

	addProgress("waiting for build VM to start...")
	if err := waitForState(buildCfg, "VirtualMachineStateRunning", 90*time.Second); err != nil {
		cleanup = false
		tail, tailErr := tailFile(buildCfg.LogFile, 80)
		if tailErr == nil && tail != "" {
			fmt.Fprint(os.Stderr, tail)
		}
		return fmt.Errorf("build VM did not reach running state: %w", err)
	}

	addProgress("waiting for build VM to acquire IP address...")
	buildIP, err := waitForBuildIP(buildCfg, 3*time.Minute)
	if err != nil {
		cleanup = false
		tail, tailErr := tailFile(buildCfg.SerialLog, 80)
		if tailErr == nil && tail != "" {
			fmt.Fprint(os.Stderr, tail)
		}
		return fmt.Errorf("build VM did not get IP: %w", err)
	}
	buildCfg.StaticIP = buildIP
	addProgress("build VM IP: %s", buildIP)

	addProgress("waiting for build VM SSH...")
	if err := waitForSSH(buildCfg, "root", 3*time.Minute); err != nil {
		cleanup = false
		tail, tailErr := tailFile(buildCfg.SerialLog, 80)
		if tailErr == nil && tail != "" {
			fmt.Fprint(os.Stderr, tail)
		}
		return fmt.Errorf("build VM SSH not ready: %w", err)
	}

	addProgress("running build script inside VM (this takes several minutes)...")
	proxyAddr, proxyStop, err := startRepoProxy(cfg.VoidRepository)
	if err != nil {
		return fmt.Errorf("failed to start repo proxy: %w", err)
	}
	defer proxyStop()
	proxyPort := proxyAddr[strings.LastIndexByte(proxyAddr, ':')+1:]
	scriptCfg := cfg
	scriptCfg.VoidRepository = "http://192.168.64.1:" + proxyPort
	script := buildVMScript(scriptCfg, cfg.VoidRepository)
	sshArgs := sshArgsForUser(buildCfg, "root")
	sshCmd := exec.Command("ssh", append(sshArgs, "bash -s")...)
	sshCmd.Stdin = strings.NewReader(script)
	stdout, _ := sshCmd.StdoutPipe()
	stderr, _ := sshCmd.StderrPipe()
	if err := sshCmd.Start(); err != nil {
		return fmt.Errorf("failed to start build script: %w", err)
	}
	scriptLog, err := os.OpenFile(buildScriptLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open build script log: %w", err)
	}
	defer scriptLog.Close()
	scannerOut := bufio.NewScanner(stdout)
	scannerErr := bufio.NewScanner(stderr)
	go func() {
		for scannerOut.Scan() {
			line := scannerOut.Text()
			addProgress("%s", line)
			fmt.Fprintln(scriptLog, line)
		}
	}()
	go func() {
		for scannerErr.Scan() {
			line := scannerErr.Text()
			addProgress("%s", line)
			fmt.Fprintln(scriptLog, line)
		}
	}()
	if err := sshCmd.Wait(); err != nil {
		addProgress("build script completed with warning: %v (continuing)", err)
		fmt.Fprintf(scriptLog, "build script exit: %v (continuing)\n", err)
	}

	addProgress("extracting kernel and initramfs from build VM...")
	findKernel := "find /mnt/target/boot -maxdepth 1 -type f \\( -name 'vmlinux-*' -o -name 'Image*' -o -name 'vmlinuz-*' \\) | sort | tail -1"
	kernelOut, err := exec.Command("ssh", append(sshArgsForUser(buildCfg, "root"), findKernel)...).Output()
	if err != nil {
		return fmt.Errorf("failed to find kernel in build VM: %w", err)
	}
	kernelPath := strings.TrimSpace(string(kernelOut))
	if kernelPath == "" {
		return fmt.Errorf("no kernel found in build VM /mnt/target/boot")
	}

	findInitrd := "find /mnt/target/boot -maxdepth 1 -type f -name 'initramfs-*.img' | sort | tail -1"
	initrdOut, err := exec.Command("ssh", append(sshArgsForUser(buildCfg, "root"), findInitrd)...).Output()
	if err != nil {
		return fmt.Errorf("failed to find initrd in build VM: %w", err)
	}
	initrdPath := strings.TrimSpace(string(initrdOut))
	if initrdPath == "" {
		return fmt.Errorf("no initramfs found in build VM /mnt/target/boot")
	}

	if err := copyRemoteFile(buildCfg, kernelPath, cfg.KernelPath); err != nil {
		return fmt.Errorf("failed to copy kernel: %w", err)
	}
	if err := copyRemoteFile(buildCfg, initrdPath, cfg.InitrdPath); err != nil {
		return fmt.Errorf("failed to copy initrd: %w", err)
	}

	addProgress("stopping build VM...")
	Stop(buildCfg)

	addProgress("renaming disk image...")
	if err := os.Rename(targetDisk, cfg.DiskPath); err != nil {
		return fmt.Errorf("failed to rename disk: %w", err)
	}

	addProgress("Void Linux VM disk built successfully")
	return nil
}

var buildIPRegexp = regexp.MustCompile(`inet (\d+\.\d+\.\d+\.\d+)/`)

func extractHost(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.IndexAny(s, "/:"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func waitForBuildIP(buildCfg Config, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(buildCfg.SerialLog)
		if err == nil {
			matches := buildIPRegexp.FindAllStringSubmatch(string(data), -1)
			for _, m := range matches {
				ip := m[1]
				if ip != "127.0.0.1" {
					return ip, nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("timed out waiting for IP in serial log")
}

func startRepoProxy(repoURL string) (string, func(), error) {
	target, err := url.Parse(repoURL)
	if err != nil {
		return "", nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return "", nil, err
	}
	addr := ln.Addr().String()
	addProgress("starting repository proxy on %s ...", addr)
	go http.Serve(ln, proxy)
	stop := func() { ln.Close() }
	return addr, stop, nil
}

func injectGuestConfig(buildCfg Config, cfg Config) error {
	pubKey, err := os.ReadFile(cfg.SSHPublicKey)
	if err != nil {
		return fmt.Errorf("failed to read SSH public key: %w", err)
	}
	escapedKey := shellQuote(strings.TrimSpace(string(pubKey)))

	script := fmt.Sprintf(`#!/bin/sh
set -e

mkdir -p /mnt/target/home/%s/.ssh
echo %s > /mnt/target/home/%s/.ssh/authorized_keys
chmod 700 /mnt/target/home/%s/.ssh
chmod 600 /mnt/target/home/%s/.ssh/authorized_keys

mkdir -p /mnt/target/home/%s/.config/fish/conf.d
cat > /mnt/target/home/%s/.config/fish/conf.d/vmctl-session.fish << 'FEOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    if string match -q /dev/tty1 (tty 2>/dev/null)
      exec /usr/local/bin/vmctl-session
    end
  end
end
FEOF

cat > /mnt/target/home/%s/.bash_profile << 'BEOF'
export XDG_RUNTIME_DIR="\${HOME}/.local/run"
mkdir -p "\${XDG_RUNTIME_DIR}"
chmod 700 "\${XDG_RUNTIME_DIR}"
if [ -z "\${WAYLAND_DISPLAY:-}" ] && [ -z "\${DISPLAY:-}" ] && [ "\$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
BEOF

chown -R %s:%s /mnt/target/home/%s 2>/dev/null || true
echo "GUEST_CONFIG_INJECTED"
`,
		cfg.GuestUser, escapedKey, cfg.GuestUser, cfg.GuestUser, cfg.GuestUser,
		cfg.GuestUser,
		cfg.GuestUser,
		cfg.GuestUser, cfg.GuestUser, cfg.GuestUser, cfg.GuestUser)

	cmd := exec.Command("ssh", append(sshArgsForUser(buildCfg, "root"), "bash -s")...)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("injectGuestConfig failed: %w\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "GUEST_CONFIG_INJECTED") {
		return fmt.Errorf("injectGuestConfig did not complete:\n%s", string(out))
	}
	addProgress("guest configuration injected successfully")
	return nil
}

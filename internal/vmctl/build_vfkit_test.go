package vmctl

import (
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateCpioInitrd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cpio test in short mode")
	}
	if _, err := exec.LookPath("cpio"); err != nil {
		t.Skip("cpio not found")
	}
	if _, err := exec.LookPath("gzip"); err != nil {
		t.Skip("gzip not found")
	}

	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "init"), []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "etc", "resolv.conf"), []byte("nameserver 8.8.8.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "initrd.cpio.gz")
	if err := createCpioInitrd(srcDir, outputPath); err != nil {
		t.Fatalf("createCpioInitrd failed: %v", err)
	}

	f, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("output file does not exist: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("output is not gzip-compressed: %v", err)
	}
	gz.Close()

	cmd := exec.Command("sh", "-c", "gzip -dc '"+outputPath+"' | cpio -t 2>/dev/null")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cpio -t failed: %v", err)
	}
	listing := string(out)
	t.Logf("cpio listing:\n%s", listing)
	if !strings.Contains(listing, "init") && !strings.Contains(listing, "./init") {
		t.Fatalf("cpio listing does not contain 'init':\n%s", listing)
	}
	if !strings.Contains(listing, "resolv.conf") {
		t.Fatalf("cpio listing does not contain 'resolv.conf':\n%s", listing)
	}
}

func TestPrepareBuildRootfs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rootfs test in short mode")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not found")
	}

	rootfsSrc := t.TempDir()
	for _, d := range []string{"etc", "bin", "root", "sbin", "etc/sv/sshd", "etc/sv/dbus", "etc/runit/runsvdir/default"} {
		if err := os.MkdirAll(filepath.Join(rootfsSrc, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(rootfsSrc, "bin", "sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(t.TempDir(), "rootfs.tar.xz")
	cmd := exec.Command("tar", "-cJf", tarPath, "-C", rootfsSrc, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating test tarball: %v\n%s", err, out)
	}

	sshKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItestkey test@host"
	pubKeyPath := filepath.Join(t.TempDir(), "id_ed25519.pub")
	if err := os.WriteFile(pubKeyPath, []byte(sshKey+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		BaseImage:      tarPath,
		SSHPublicKey:   pubKeyPath,
		VoidRepository: "https://repo-default.voidlinux.org",
		DNSServers:     "1.1.1.1,8.8.8.8",
		Name:           "test-vm",
	}

	rootfsDir := t.TempDir()
	if err := prepareBuildRootfs(cfg, rootfsDir); err != nil {
		t.Fatalf("prepareBuildRootfs failed: %v", err)
	}

	repoConf := filepath.Join(rootfsDir, "etc", "xbps.d", "00-vmctl-repository.conf")
	data, err := os.ReadFile(repoConf)
	if err != nil {
		t.Fatalf("xbps repo conf missing: %v", err)
	}
	wantRepo := "repository=https://repo-default.voidlinux.org/current/aarch64\n"
	if string(data) != wantRepo {
		t.Fatalf("xbps repo conf = %q, want %q", string(data), wantRepo)
	}

	authKeys, err := os.ReadFile(filepath.Join(rootfsDir, "root", ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("authorized_keys missing: %v", err)
	}
	if strings.TrimSpace(string(authKeys)) != sshKey {
		t.Fatalf("authorized_keys = %q, want %q", strings.TrimSpace(string(authKeys)), sshKey)
	}

	sshdConf := filepath.Join(rootfsDir, "etc", "ssh", "sshd_config.d", "99-vmctl.conf")
	if _, err := os.Stat(sshdConf); err != nil {
		t.Fatalf("sshd_config.d/99-vmctl.conf missing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootfsDir, "etc", "resolv.conf")); err != nil {
		t.Fatalf("resolv.conf missing: %v", err)
	}

	initPath := filepath.Join(rootfsDir, "init")
	initInfo, err := os.Stat(initPath)
	if err != nil {
		t.Fatalf("init missing: %v", err)
	}
	if initInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("init not executable: mode=%o", initInfo.Mode())
	}

	hostname, err := os.ReadFile(filepath.Join(rootfsDir, "etc", "hostname"))
	if err != nil {
		t.Fatalf("hostname missing: %v", err)
	}
	if strings.TrimSpace(string(hostname)) != "test-vm" {
		t.Fatalf("hostname = %q, want %q", strings.TrimSpace(string(hostname)), "test-vm")
	}
}

func TestDownloadBuildKernel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping kernel test in short mode")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not found")
	}

	tmpDir := t.TempDir()
	bootDir := filepath.Join(tmpDir, "boot")
	if err := os.MkdirAll(bootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	kernelContent := []byte("fake kernel data")
	if err := os.WriteFile(filepath.Join(bootDir, "vmlinuz-6.12.18"), kernelContent, 0o644); err != nil {
		t.Fatal(err)
	}

	xbpsPath := filepath.Join(t.TempDir(), "linux-build.xbps")
	tarCmd := exec.Command("tar", "-cf", xbpsPath, "-C", tmpDir, "boot")
	if out, err := tarCmd.CombinedOutput(); err != nil {
		t.Fatalf("creating test xbps: %v\n%s", err, out)
	}

	stateDir := t.TempDir()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		StateDir:       stateDir,
		BuildKernelURL: "file://" + xbpsPath,
	}

	kernelPath, err := downloadBuildKernel(cfg)
	if err != nil {
		t.Fatalf("downloadBuildKernel failed: %v", err)
	}

	if kernelPath != filepath.Join(stateDir, "build-vmlinuz") {
		t.Fatalf("kernel path = %q, want %q", kernelPath, filepath.Join(stateDir, "build-vmlinuz"))
	}

	got, err := os.ReadFile(kernelPath)
	if err != nil {
		t.Fatalf("reading extracted kernel: %v", err)
	}
	if string(got) != string(kernelContent) {
		t.Fatalf("kernel content = %q, want %q", string(got), string(kernelContent))
	}

	kernelPath2, err := downloadBuildKernel(cfg)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if kernelPath2 != kernelPath {
		t.Fatalf("cached path = %q, want %q", kernelPath2, kernelPath)
	}
}

func TestBuildVMScript(t *testing.T) {
	tmpKey := filepath.Join(t.TempDir(), "test.pub")
	os.WriteFile(tmpKey, []byte("ssh-ed25519 AAAAtest"), 0o644)

	cfg := Config{
		VoidRepository: "https://repo-default.voidlinux.org",
		Name:           "test-vm",
		DefaultShell:   "zsh",
		GuestUser:      "devuser",
		RootPassword:   "rootpw",
		GuestPassword:  "guestpw",
		MAC:            "52:54:00:64:00:10",
		StaticIP:       "192.168.64.10",
		CIDR:           24,
		Gateway:        "192.168.64.1",
		DNSServers:     "1.1.1.1,8.8.8.8",
		Timezone:       "Australia/Sydney",
		SSHPublicKey:   tmpKey,
	}

	script, err := buildVMScript(cfg, cfg.VoidRepository); if err != nil { t.Fatal(err) }

	wantRepo := "https://repo-default.voidlinux.org/current"
	if !strings.Contains(script, wantRepo) {
		t.Fatalf("script does not contain repo URL %q", wantRepo)
	}
	if !strings.Contains(script, "devuser") {
		t.Fatal("script does not contain guest username")
	}
	if !strings.Contains(script, "52:54:00:64:00:10") {
		t.Fatal("script does not contain MAC address")
	}
	if strings.Contains(script, "{{") {
		t.Fatalf("script contains unresolved template markers:\n%s", script)
	}
	if !strings.Contains(script, "1.1.1.1;8.8.8.8") {
		t.Fatal("script does not contain DNS servers (semicolon-separated)")
	}
}

func TestDirSizeMB(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(dir, "file"+string(rune('0'+i))), make([]byte, 1024*1024), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := dirSizeMB(dir)
	want := 3.0
	if got < want-0.01 || got > want+0.01 {
		t.Fatalf("dirSizeMB = %.2f, want %.2f", got, want)
	}
}

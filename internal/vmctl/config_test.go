package vmctl

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vmctl.env")
	body := "VM_NAME=test-vm\n# comment\nVM_GUI=0\nVM_BASE_IMAGE=\"/tmp/test.img\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	env, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env["VM_NAME"] != "test-vm" {
		t.Fatalf("unexpected VM_NAME: %q", env["VM_NAME"])
	}
	if env["VM_GUI"] != "0" {
		t.Fatalf("unexpected VM_GUI: %q", env["VM_GUI"])
	}
	if env["VM_BASE_IMAGE"] != "/tmp/test.img" {
		t.Fatalf("unexpected VM_BASE_IMAGE: %q", env["VM_BASE_IMAGE"])
	}
}

func TestIsCompressedRawImage(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/tmp/foo.img.xz", want: true},
		{path: "/tmp/foo.raw.xz", want: true},
		{path: "/tmp/foo.img", want: false},
		{path: "/tmp/foo.iso", want: false},
	}

	for _, tt := range tests {
		if got := isCompressedRawImage(tt.path); got != tt.want {
			t.Fatalf("%s: got %v want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsVoidLinuxRootfsTarball(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/tmp/void-aarch64-ROOTFS-20250202.tar.xz", want: true},
		{path: "/tmp/void-live-aarch64-20250202-base.iso", want: false},
		{path: "/tmp/ArchLinuxARM-aarch64-latest.tar.gz", want: false},
	}

	for _, tt := range tests {
		if got := isVoidLinuxRootfsTarball(tt.path); got != tt.want {
			t.Fatalf("%s: got %v want %v", tt.path, got, tt.want)
		}
	}
}

func TestPrepareDiskCompressedBaseImage(t *testing.T) {
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz not available")
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}

	dir := t.TempDir()
	rawBase := filepath.Join(dir, "base.img")
	xzBase := rawBase + ".xz"
	stateDir := filepath.Join(dir, "state")
	diskPath := filepath.Join(stateDir, "disk.img")

	f, err := os.OpenFile(rawBase, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(4 * 1024 * 1024); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("xz", "-z", "-k", rawBase)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		RepoRoot:  dir,
		StateDir:  stateDir,
		DiskPath:  diskPath,
		BaseImage: xzBase,
		DiskSize:  "8M",
	}

	if _, err := prepareDisk(cfg); err != nil {
		t.Fatalf("prepareDisk failed: %v", err)
	}

	info, err := os.Stat(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 8*1024*1024 {
		t.Fatalf("unexpected disk size: got %d want %d", info.Size(), 8*1024*1024)
	}
}

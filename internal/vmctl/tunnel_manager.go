package vmctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func buildSSHCommand(cfg Config, tunnel Tunnel) *exec.Cmd {
	args := []string{"-N"}

	if tunnel.Type == TunnelTypeLocal {
		remoteHost := tunnel.RemoteHost
		if remoteHost == "" {
			remoteHost = "localhost"
		}
		args = append(args, "-L", fmt.Sprintf("%d:%s:%d", tunnel.LocalPort, remoteHost, tunnel.RemotePort))
	} else if tunnel.Type == TunnelTypeRemote {
		args = append(args, "-R", fmt.Sprintf("%d:localhost:%d", tunnel.RemotePort, tunnel.LocalPort))
	}

	args = append(args,
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
	)

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

	args = append(args, cfg.SSHUser+"@"+cfg.StaticIP)
	return exec.Command("ssh", args...)
}

func StartTunnel(cfg Config, tunnel Tunnel) error {
	if IsTunnelRunning(cfg, tunnel) {
		return nil
	}

	cmd := buildSSHCommand(cfg, tunnel)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	pidFile := tunnelPIDFile(cfg, tunnel.ID)
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return err
	}

	_ = cmd.Process.Release()
	return nil
}

func StopTunnel(cfg Config, tunnel Tunnel) error {
	pidFile := tunnelPIDFile(cfg, tunnel.ID)
	pid, err := readPID(pidFile)
	if err != nil {
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidFile)
		return nil
	}

	_ = proc.Signal(syscall.SIGTERM)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := proc.Signal(syscall.Signal(0))
		if err != nil {
			_ = os.Remove(pidFile)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = proc.Signal(syscall.SIGKILL)
	_ = os.Remove(pidFile)
	return nil
}

func IsTunnelRunning(cfg Config, tunnel Tunnel) bool {
	pidFile := tunnelPIDFile(cfg, tunnel.ID)
	pid, err := readPID(pidFile)
	if err != nil {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func tunnelPIDFile(cfg Config, tunnelID string) string {
	return filepath.Join(cfg.StateDir, "tunnels", tunnelID+".pid")
}

# VM Lifecycle Integration Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate SSH tunnel lifecycle with VM start/stop operations — stop all tunnels when VM stops, auto-start tunnels marked `auto_start: true` when VM starts.

**Architecture:** Two new functions (`StopAllTunnels`, `StartAutoTunnels`) in `tunnel_manager.go` handle bulk tunnel operations. `vm.go` calls them at the right lifecycle points. `Destroy` in `inspect.go` already calls `Stop`, so no change needed there.

**Tech Stack:** Go, existing vmctl tunnel/VM infrastructure.

---

## File Map

| File | Responsibility |
|------|---------------|
| `internal/vmctl/tunnel_manager.go` | Add `StopAllTunnels` and `StartAutoTunnels` functions |
| `internal/vmctl/tunnel_manager_test.go` | Unit tests for the two new functions |
| `internal/vmctl/vm.go` | Call `StartAutoTunnels` in `Start`, call `StopAllTunnels` in `Stop` |

---

## Task 1: Add `StopAllTunnels` and `StartAutoTunnels` to `tunnel_manager.go`

**Files:**
- Modify: `internal/vmctl/tunnel_manager.go`

- [ ] **Step 1: Add `StopAllTunnels` function**

Append the following function to `internal/vmctl/tunnel_manager.go` (after `tunnelPIDFile`):

```go
// StopAllTunnels stops all active tunnels.
// It loads the tunnel config and stops each tunnel that is currently running.
// Errors are collected and a summary error is returned if any failed.
func StopAllTunnels(cfg Config) error {
	tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
	if err != nil {
		return fmt.Errorf("load tunnel config: %w", err)
	}

	var errs []error
	for _, tunnel := range tc.Tunnels {
		if !IsTunnelRunning(cfg, tunnel) {
			continue
		}
		if err := StopTunnel(cfg, tunnel); err != nil {
			errs = append(errs, fmt.Errorf("stop tunnel %q: %w", tunnel.ID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop %d tunnel(s): %v", len(errs), errs)
	}
	return nil
}
```

- [ ] **Step 2: Add `StartAutoTunnels` function**

Append the following function right after `StopAllTunnels`:

```go
// StartAutoTunnels starts all tunnels marked with auto_start and enabled.
func StartAutoTunnels(cfg Config) error {
	tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
	if err != nil {
		return fmt.Errorf("load tunnel config: %w", err)
	}

	var started int
	var errs []error
	for _, tunnel := range tc.Tunnels {
		if !tunnel.AutoStart || !tunnel.Enabled {
			continue
		}
		if IsTunnelRunning(cfg, tunnel) {
			logf("tunnel %q already running", tunnel.ID)
			continue
		}
		if err := StartTunnel(cfg, tunnel); err != nil {
			logf("failed to start tunnel %q: %v", tunnel.ID, err)
			errs = append(errs, fmt.Errorf("start tunnel %q: %w", tunnel.ID, err))
			continue
		}
		logf("started tunnel %q", tunnel.ID)
		started++
	}

	if started > 0 {
		logf("auto-started %d tunnel(s)", started)
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to start %d tunnel(s): %v", len(errs), errs)
	}
	return nil
}
```

- [ ] **Step 3: Verify compilation**

Run:
```bash
cd /Users/zhouye/vms/vfkit-void && go build ./cmd/vmctl
```

Expected: builds cleanly with no errors.

---

## Task 2: Add unit tests for `StopAllTunnels` and `StartAutoTunnels`

**Files:**
- Modify: `internal/vmctl/tunnel_manager_test.go`

- [ ] **Step 1: Write test for `StopAllTunnels` with no config file**

Append to `internal/vmctl/tunnel_manager_test.go`:

```go
func TestStopAllTunnelsNoConfig(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
	}
	// No tunnel config file exists — should return nil (nothing to stop).
	if err := StopAllTunnels(cfg); err != nil {
		t.Fatalf("expected nil error when no config, got %v", err)
	}
}
```

- [ ] **Step 2: Write test for `StopAllTunnels` with dead tunnel**

Append:

```go
func TestStopAllTunnelsWithDeadTunnel(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
		StateDir: t.TempDir(),
	}

	// Create a tunnel config with one tunnel whose PID file references a dead process.
	tc := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{ID: "dead", Name: "Dead Tunnel", Enabled: true},
		},
	}
	path := tunnelConfigPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveTunnelConfig(path, tc); err != nil {
		t.Fatal(err)
	}

	// Write a non-existent PID.
	pidFile := tunnelPIDFile(cfg, "dead")
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should succeed because the tunnel is not actually running.
	if err := StopAllTunnels(cfg); err != nil {
		t.Fatalf("expected nil error for dead tunnel, got %v", err)
	}
}
```

- [ ] **Step 3: Write test for `StartAutoTunnels` with no config file**

Append:

```go
func TestStartAutoTunnelsNoConfig(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
	}
	if err := StartAutoTunnels(cfg); err != nil {
		t.Fatalf("expected nil error when no config, got %v", err)
	}
}
```

- [ ] **Step 4: Write test for `StartAutoTunnels` skips non-auto-start tunnels**

Append:

```go
func TestStartAutoTunnelsSkipsNonAutoStart(t *testing.T) {
	cfg := Config{
		RepoRoot: t.TempDir(),
		StateDir: t.TempDir(),
		StaticIP: "192.168.64.10",
		SSHUser:  "dev",
	}

	tc := TunnelConfig{
		Version: 1,
		Tunnels: []Tunnel{
			{ID: "manual", Name: "Manual Tunnel", Enabled: true, AutoStart: false},
			{ID: "disabled", Name: "Disabled Tunnel", Enabled: false, AutoStart: true},
		},
	}
	path := tunnelConfigPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveTunnelConfig(path, tc); err != nil {
		t.Fatal(err)
	}

	// Should succeed without starting anything.
	if err := StartAutoTunnels(cfg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
```

- [ ] **Step 5: Run tests**

Run:
```bash
cd /Users/zhouye/vms/vfkit-void && go test ./internal/vmctl -run "TestStopAllTunnels|TestStartAutoTunnels" -v
```

Expected: all 4 new tests PASS.

---

## Task 3: Integrate tunnel lifecycle into `vm.go`

**Files:**
- Modify: `internal/vmctl/vm.go`

- [ ] **Step 1: Call `StartAutoTunnels` in `Start` after bootstrap**

In `internal/vmctl/vm.go`, find the `Start` function. After the bootstrap block (the `if voidBootstrapCandidate && !fileExists(cfg.BootstrapMarker) { ... }` block) and before the final `return Status(cfg)`, add:

```go
	if err := StartAutoTunnels(cfg); err != nil {
		logf("auto-start tunnels: %v", err)
	}
```

The end of `Start` should look like:

```go
	if voidBootstrapCandidate && !fileExists(cfg.BootstrapMarker) {
		logf("waiting for SSH so first-boot bootstrap can finish")
		if err := waitForSSH(cfg, cfg.SSHUser, 5*time.Minute); err != nil {
			return err
		}
		if err := Bootstrap(cfg); err != nil {
			return err
		}
		if err := os.WriteFile(cfg.BootstrapMarker, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return err
		}
	}
	if err := StartAutoTunnels(cfg); err != nil {
		logf("auto-start tunnels: %v", err)
	}
	return Status(cfg)
}
```

- [ ] **Step 2: Call `StopAllTunnels` in `Stop` after VM stops**

In `internal/vmctl/vm.go`, find the `Stop` function. After the VM is confirmed stopped (after `logf("VM stopped")` and before `return nil`), add:

```go
		if err := StopAllTunnels(cfg); err != nil {
			logf("stop tunnels: %v", err)
		}
```

The relevant section should become:

```go
		if !running {
			_ = os.Remove(cfg.PIDFile)
			_ = os.Remove(cfg.RestSocket)
			logf("VM stopped")
			if err := StopAllTunnels(cfg); err != nil {
				logf("stop tunnels: %v", err)
			}
			return nil
		}
```

- [ ] **Step 3: Verify compilation**

Run:
```bash
cd /Users/zhouye/vms/vfkit-void && go build ./cmd/vmctl
```

Expected: builds cleanly.

---

## Task 4: Run full test suite

- [ ] **Step 1: Run all vmctl tests**

```bash
cd /Users/zhouye/vms/vfkit-void && go test ./internal/vmctl/... -v
```

Expected: all tests PASS.

- [ ] **Step 2: Build the CLI**

```bash
cd /Users/zhouye/vms/vfkit-void && go build ./cmd/vmctl
```

Expected: builds cleanly.

---

## Task 5: Commit

- [ ] **Step 1: Stage changes**

```bash
cd /Users/zhouye/vms/vfkit-void && git add internal/vmctl/tunnel_manager.go internal/vmctl/tunnel_manager_test.go internal/vmctl/vm.go
```

- [ ] **Step 2: Commit**

```bash
cd /Users/zhouye/vms/vfkit-void && git commit -m "feat(vmctl): integrate tunnel lifecycle with VM start/stop (Task 5)

- Add StopAllTunnels to stop all active tunnels
- Add StartAutoTunnels to auto-start tunnels marked auto_start=true
- Call StartAutoTunnels after VM bootstrap in Start
- Call StopAllTunnels after VM stops in Stop
- Add unit tests for both new functions"
```

---

## Spec Coverage Checklist

| Requirement | Task |
|------------|------|
| `StopAllTunnels` function | Task 1 |
| `StartAutoTunnels` function | Task 1 |
| Stop all tunnels when VM stops | Task 3, Step 2 |
| Auto-start tunnels when VM starts | Task 3, Step 1 |
| `Destroy` already calls `Stop` — no extra change needed | N/A (already true) |
| Unit tests for new functions | Task 2 |
| Compilation passes | Task 1, 3, 4 |
| Full test suite passes | Task 4 |

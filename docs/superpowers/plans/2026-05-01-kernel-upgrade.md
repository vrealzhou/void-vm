# Kernel Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one-click kernel upgrade to the Web UI that upgrades the kernel inside the VM via SSH, pulls the new boot assets to the host, and restarts the VM.

**Architecture:** New `UpgradeKernel` function in `vm.go` runs xbps-install + xbps-reconfigure via SSH, finds the latest kernel/initrd in `/boot`, pipes them back to the host with atomic file writes, then stops and starts the VM. A new API endpoint and UI button expose this to the user.

**Tech Stack:** Go (Echo v5), SSH, xbps (Void Linux package manager)

---

### Task 1: Add `UpgradeKernel` backend function

**Files:**
- Modify: `internal/vmctl/vm.go` (add function after `BootstrapSetup`)

- [ ] **Step 1: Write the `UpgradeKernel` function**

Add this function to `internal/vmctl/vm.go` after the `BootstrapSetup` function (after line 201):

```go
func UpgradeKernel(cfg Config) (string, error) {
	if err := waitForSSH(cfg, cfg.SSHUser, 60*time.Second); err != nil {
		return "", fmt.Errorf("SSH not ready: %w", err)
	}

	upgradeCmd := "xbps-install -uy linux6.12 && xbps-reconfigure -f linux6.12"
	cmd := exec.Command("ssh", append(sshArgs(cfg), upgradeCmd)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
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
```

Note: requires `"path/filepath"` and `"strings"` already imported in `vm.go`.

- [ ] **Step 2: Build to verify compilation**

Run: `go build ./cmd/vmctl`
Expected: clean build, no errors

---

### Task 2: Add `POST /api/upgrade-kernel` handler and route

**Files:**
- Modify: `internal/vmctl/web_handlers.go` (add handler after `handleDestroy`)
- Modify: `internal/vmctl/web.go` (register route)

- [ ] **Step 1: Add handler to `web_handlers.go`**

Add after the `handleDestroy` function (after line 118):

```go
func handleUpgradeKernel(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		status, err := InspectVM(cfg)
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		if !status.Running {
			return jsonError(c, http.StatusServiceUnavailable, "VM is not running")
		}
		go func() {
			version, err := UpgradeKernel(cfg)
			if err != nil {
				fmt.Printf("kernel upgrade error: %v\n", err)
			} else {
				fmt.Printf("[vmctl] kernel upgraded to %s\n", version)
			}
		}()
		return jsonSuccess(c, map[string]string{"message": "kernel upgrade started"})
	}
}
```

- [ ] **Step 2: Register the route in `web.go`**

Add after the `e.POST("/api/destroy", handleDestroy(cfg))` line (line 54):

```go
		e.POST("/api/upgrade-kernel", handleUpgradeKernel(cfg))
```

- [ ] **Step 3: Build to verify**

Run: `go build ./cmd/vmctl`
Expected: clean build

---

### Task 3: Add kernel version to status display

**Files:**
- Modify: `internal/vmctl/inspect.go` (add `KernelVersion` field to `VMStatus`)
- Modify: `internal/vmctl/inspect.go` (populate field in `InspectVM`)

- [ ] **Step 1: Add `KernelVersion` field**

In `internal/vmctl/inspect.go`, add to the `VMStatus` struct (after line 20):

```go
	KernelVersion string
```

- [ ] **Step 2: Populate the field in `InspectVM`**

In the `InspectVM` function, after the line `status.BootstrapDone = fileExists(cfg.BootstrapMarker)` (line 48), add:

```go
	if bootAssetsExist(cfg) {
		status.KernelVersion = filepath.Base(cfg.KernelPath)
	}
```

Add `"path/filepath"` to the imports in `inspect.go` if not present.

- [ ] **Step 3: Build and run tests**

Run: `go build ./cmd/vmctl && go test ./internal/vmctl/...`
Expected: clean build, all tests pass

---

### Task 4: Add "Upgrade Kernel" button to Web UI

**Files:**
- Modify: `web/static/index.html` (add button)
- Modify: `web/static/app.js` (add element reference, click handler, status display)

- [ ] **Step 1: Add button to `index.html`**

In the Actions button-grid div (line 19, after the Destroy button), add:

```html
                    <button id="btn-upgrade-kernel" class="btn">Upgrade Kernel</button>
```

- [ ] **Step 2: Add element reference in `app.js`**

In the `els` object (after line 22, the `btnDestroy` line), add:

```js
    btnUpgradeKernel: document.getElementById('btn-upgrade-kernel'),
```

- [ ] **Step 3: Update `updateButtons` to handle the new button**

In the `updateButtons` function, add after `els.btnDestroy.disabled = busy;` (after line 66):

```js
    els.btnUpgradeKernel.disabled = !bootstrapDone || !running || busy;
```

Also add `els.btnUpgradeKernel` to the disabled array in `setBusy`:

```js
    [els.btnBootstrap, els.btnStart, els.btnStop, els.btnDestroy, els.btnUpgradeKernel].forEach(btn => {
```

- [ ] **Step 4: Add kernel version to status display**

In the `formatOverview` function, add after the `SSH:` line (after line 92):

```js
    if (status.KernelVersion) lines.push(`Kernel: ${status.KernelVersion}`);
```

- [ ] **Step 5: Add click handler**

Add before the `refreshStatus()` call (before line 385):

```js
els.btnUpgradeKernel.onclick = () => {
    if (!confirm('Upgrade the VM kernel? This will restart the VM.')) return;
    setBusy(true); setAction('Upgrading kernel...');
    API.post('/api/upgrade-kernel')
        .then(() => showToast('Kernel upgrade started'))
        .catch(err => { showToast('Kernel upgrade failed: ' + err.message, 'error'); setAction('Kernel upgrade failed: ' + err.message); })
        .finally(() => setBusy(false));
};
```

- [ ] **Step 6: Build and verify**

Run: `go build ./cmd/vmctl`
Expected: clean build

---

### Task 5: Commit

- [ ] **Step 1: Commit all changes**

```bash
git add internal/vmctl/vm.go internal/vmctl/web_handlers.go internal/vmctl/web.go internal/vmctl/inspect.go web/static/index.html web/static/app.js docs/superpowers/specs/2026-05-01-kernel-upgrade-design.md
git commit -m "feat: add one-click kernel upgrade from Web UI"
```

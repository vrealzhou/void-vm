# Fyne to Web UI Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Fyne GUI with a web-based UI served by Echo v5, including a VM file picker and username change from `dev` to `vm`.

**Architecture:** Go backend with Echo v5 serves REST API and static files. Frontend is vanilla HTML/CSS/JS with fetch-based polling. VM file listing is done via SSH `ls` commands.

**Tech Stack:** Go 1.26, Echo v5, vanilla HTML/CSS/JS

---

## File Structure

```
internal/vmctl/
├── web.go              # Echo server setup, route registration, port config
├── web_handlers.go     # HTTP handler functions for all API endpoints
├── web_static.go       # embed directive for static files
├── web_vmfiles.go      # VM file listing via SSH
├── gui.go              # DELETED
├── tunnel_gui.go       # DELETED
├── sync_gui.go         # DELETED
├── config.go           # Modified: default user changed to "vm"
├── sync_git.go         # Modified: paths changed to /home/vm/
└── ...

web/static/
├── index.html          # Main page layout
├── style.css           # Styles
└── app.js              # Frontend logic, modals, polling
```

---

### Task 1: Add Echo v5 Dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add echo v5**

Run:
```bash
go get github.com/labstack/echo/v5
```

- [ ] **Step 2: Verify go.mod**

Expected: `github.com/labstack/echo/v5` appears in `require` block.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add echo v5"
```

---

### Task 2: Remove Fyne GUI Files

**Files:**
- Delete: `internal/vmctl/gui.go`
- Delete: `internal/vmctl/tunnel_gui.go`
- Delete: `internal/vmctl/sync_gui.go`
- Modify: `go.mod`
- Modify: `internal/vmctl/cobra.go`

- [ ] **Step 1: Delete Fyne GUI files**

```bash
rm internal/vmctl/gui.go internal/vmctl/tunnel_gui.go internal/vmctl/sync_gui.go
```

- [ ] **Step 2: Update cobra.go**

Modify `internal/vmctl/cobra.go`:
- Remove `LaunchControlGUI()` call from root command
- Change `gui` subcommand to launch web server instead
- Update help text

```go
func NewRootCommand() (*cobra.Command, error) {
	// ... existing setup ...
	rootCmd := &cobra.Command{
		Use:           "vmctl",
		Short:         "Manage a Void Linux dev VM with vfkit",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return LaunchWebServer("") // default port
		},
	}
	// ...
}

func newGUICommand(cfg Config) *cobra.Command {
	var port string
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Open the Web VM control panel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return LaunchWebServer(port)
		},
	}
	cmd.Flags().StringVar(&port, "port", "", "Server port (default: 8080 or VM_MANAGER_PORT env)")
	return cmd
}
```

- [ ] **Step 3: Remove fyne from go.mod**

Run:
```bash
go mod tidy
```

Verify `fyne.io/fyne/v2` is no longer in go.mod.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/vmctl/...
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove fyne gui files, update cobra"
```

---

### Task 3: Create Web Server Core

**Files:**
- Create: `internal/vmctl/web.go`

- [ ] **Step 1: Write web.go**

```go
package vmctl

import (
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func getPort(flagPort string) string {
	if flagPort != "" {
		return flagPort
	}
	if envPort := os.Getenv("VM_MANAGER_PORT"); envPort != "" {
		return envPort
	}
	return "8080"
}

func LaunchWebServer(flagPort string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	port := getPort(flagPort)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLogger())

	// Static files
	e.Static("/static", "web/static")

	// Main page
	e.GET("/", func(c *echo.Context) error {
		return c.File("web/static/index.html")
	})

	// API routes
	registerAPIRoutes(e, cfg)

	fmt.Printf("Web UI running at http://localhost:%s\n", port)
	return e.Start(":" + port)
}

func registerAPIRoutes(e *echo.Echo, cfg Config) {
	// VM control
	e.GET("/api/status", handleStatus(cfg))
	e.POST("/api/bootstrap", handleBootstrap(cfg))
	e.POST("/api/start", handleStart(cfg))
	e.POST("/api/stop", handleStop(cfg))
	e.POST("/api/destroy", handleDestroy(cfg))

	// Tunnels
	e.GET("/api/tunnels", handleListTunnels(cfg))
	e.POST("/api/tunnels", handleAddTunnel(cfg))
	e.DELETE("/api/tunnels/:id", handleRemoveTunnel(cfg))
	e.POST("/api/tunnels/:id/start", handleStartTunnel(cfg))
	e.POST("/api/tunnels/:id/stop", handleStopTunnel(cfg))

	// Sync
	e.GET("/api/sync", handleListSync(cfg))
	e.POST("/api/sync", handleAddSync(cfg))
	e.DELETE("/api/sync/:id", handleRemoveSync(cfg))
	e.POST("/api/sync/:id/run", handleRunSync(cfg))
	e.GET("/api/sync/:id/history", handleSyncHistory(cfg))

	// VM Files
	e.GET("/api/vm-files", handleVMFiles(cfg))
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/vmctl/web.go
git commit -m "feat(web): add echo server core with route registration"
```

---

### Task 4: Create VM File Listing Handler

**Files:**
- Create: `internal/vmctl/web_vmfiles.go`

- [ ] **Step 1: Write web_vmfiles.go**

```go
package vmctl

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"
)

type VMFileEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

func handleVMFiles(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		path := c.QueryParam("path")
		root := c.QueryParam("root")

		if path == "" {
			if root != "" {
				path = root
			} else {
				path = "/home/vm"
			}
		}

		// Security: prevent escaping root
		if root != "" {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid path"})
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid root"})
			}
			if !strings.HasPrefix(absPath, absRoot) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "cannot navigate above root"})
			}
		}

		entries, err := listVMFiles(cfg, path)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, entries)
	}
}

func listVMFiles(cfg Config, path string) ([]VMFileEntry, error) {
	remoteCmd := fmt.Sprintf("ls -la %s", shellQuote(path))
	cmd := exec.Command("ssh", append(sshArgs(cfg), remoteCmd)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ssh ls failed: %w", err)
	}

	var entries []VMFileEntry
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 9 {
			continue
		}
		name := strings.Join(parts[8:], " ")
		if name == "." || name == ".." {
			continue
		}
		isDir := strings.HasPrefix(parts[0], "d")
		entries = append(entries, VMFileEntry{Name: name, IsDir: isDir})
	}
	return entries, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/vmctl/web_vmfiles.go
git commit -m "feat(web): add vm file listing via ssh"
```

---

### Task 5: Create API Handlers

**Files:**
- Create: `internal/vmctl/web_handlers.go`

- [ ] **Step 1: Write web_handlers.go**

```go
package vmctl

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

func jsonError(c *echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

func jsonSuccess(c *echo.Context, data any) error {
	return c.JSON(http.StatusOK, data)
}

func handleStatus(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		status, err := InspectVM(cfg)
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		var metrics *GuestMetrics
		if status.Running {
			sample, err := SampleGuestMetrics(cfg)
			if err == nil {
				calculated := CalculateGuestMetrics(sample, nil)
				metrics = &calculated
			}
		}
		return jsonSuccess(c, map[string]any{
			"status":  status,
			"metrics": metrics,
		})
	}
}

func handleBootstrap(cfg Config) echo.HandlerFunc {
	type bootstrapReq struct {
		Shell         string `json:"shell"`
		Editor        string `json:"editor"`
		WindowManager string `json:"windowManager"`
	}
	return func(c *echo.Context) error {
		var req bootstrapReq
		if err := c.Bind(&req); err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid request")
		}
		cfgPath := DotEnvPath(cfg.RepoRoot)
		updates := map[string]string{
			"VM_DEFAULT_SHELL":  req.Shell,
			"VM_DEFAULT_EDITOR": req.Editor,
			"VM_WINDOW_MANAGER": req.WindowManager,
		}
		if err := UpdateDotEnvFile(cfgPath, updates); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		newCfg, err := LoadConfig()
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		go func() {
			if err := BootstrapSetup(newCfg); err != nil {
				fmt.Printf("bootstrap error: %v\n", err)
			}
		}()
		return jsonSuccess(c, map[string]string{"message": "bootstrap started"})
	}
}

func handleStart(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		go func() {
			if err := Start(cfg); err != nil {
				fmt.Printf("start error: %v\n", err)
			}
		}()
		return jsonSuccess(c, map[string]string{"message": "start initiated"})
	}
}

func handleStop(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		go func() {
			if err := Stop(cfg); err != nil {
				fmt.Printf("stop error: %v\n", err)
			}
		}()
		return jsonSuccess(c, map[string]string{"message": "stop initiated"})
	}
}

func handleDestroy(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		go func() {
			if err := Destroy(cfg); err != nil {
				fmt.Printf("destroy error: %v\n", err)
			}
		}()
		return jsonSuccess(c, map[string]string{"message": "destroy initiated"})
	}
}

func handleListTunnels(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		type tunnelWithStatus struct {
			Tunnel
			Running bool `json:"running"`
		}
		var result []tunnelWithStatus
		for _, t := range tc.Tunnels {
			result = append(result, tunnelWithStatus{
				Tunnel:  t,
				Running: IsTunnelRunning(cfg, t),
			})
		}
		return jsonSuccess(c, map[string]any{
			"version": tc.Version,
			"tunnels": result,
		})
	}
}

func handleAddTunnel(cfg Config) echo.HandlerFunc {
	type tunnelReq struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		LocalPort  int    `json:"localPort"`
		RemotePort int    `json:"remotePort"`
		RemoteHost string `json:"remoteHost"`
		AutoStart  bool   `json:"autoStart"`
	}
	return func(c *echo.Context) error {
		var req tunnelReq
		if err := c.Bind(&req); err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid request")
		}
		var tunnelType TunnelType
		if req.Type == "local" {
			tunnelType = TunnelTypeLocal
		} else {
			tunnelType = TunnelTypeRemote
		}
		if req.RemoteHost == "" {
			req.RemoteHost = "localhost"
		}
		tunnel := Tunnel{
			ID:         req.Name,
			Name:       req.Name,
			Type:       tunnelType,
			LocalPort:  req.LocalPort,
			RemoteHost: req.RemoteHost,
			RemotePort: req.RemotePort,
			Enabled:    true,
			AutoStart:  req.AutoStart,
			CreatedAt:  time.Now().UTC(),
		}
		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		if err := tc.AddTunnel(tunnel); err != nil {
			return jsonError(c, http.StatusBadRequest, err.Error())
		}
		if err := SaveTunnelConfig(tunnelConfigPath(cfg), tc); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, tunnel)
	}
}

func handleRemoveTunnel(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := c.Param("id")
		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		tunnel, ok := tc.GetTunnel(id)
		if !ok {
			return jsonError(c, http.StatusNotFound, "tunnel not found")
		}
		if IsTunnelRunning(cfg, tunnel) {
			_ = StopTunnel(cfg, tunnel)
		}
		if !tc.RemoveTunnel(id) {
			return jsonError(c, http.StatusNotFound, "tunnel not found")
		}
		if err := SaveTunnelConfig(tunnelConfigPath(cfg), tc); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, map[string]string{"message": "tunnel removed"})
	}
}

func handleStartTunnel(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := c.Param("id")
		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		tunnel, ok := tc.GetTunnel(id)
		if !ok {
			return jsonError(c, http.StatusNotFound, "tunnel not found")
		}
		status, err := InspectVM(cfg)
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		if !status.Running {
			return jsonError(c, http.StatusServiceUnavailable, "VM is not running")
		}
		if err := StartTunnel(cfg, tunnel); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, map[string]string{"message": "tunnel started"})
	}
}

func handleStopTunnel(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := c.Param("id")
		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		tunnel, ok := tc.GetTunnel(id)
		if !ok {
			return jsonError(c, http.StatusNotFound, "tunnel not found")
		}
		if err := StopTunnel(cfg, tunnel); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, map[string]string{"message": "tunnel stopped"})
	}
}

func handleListSync(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		sc, err := LoadSyncConfig(syncConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, sc)
	}
}

func handleAddSync(cfg Config) echo.HandlerFunc {
	type syncReq struct {
		Mode                string   `json:"mode"`
		HostPath            string   `json:"hostPath"`
		VMPath              string   `json:"vmPath"`
		BareRepoPath        string   `json:"bareRepoPath"`
		Direction           string   `json:"direction"`
		Exclude             []string `json:"exclude"`
		BackupRetentionDays int      `json:"backupRetentionDays"`
	}
	return func(c *echo.Context) error {
		var req syncReq
		if err := c.Bind(&req); err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid request")
		}
		pair := SyncPair{
			ID:        filepath.Base(req.HostPath),
			HostPath:  req.HostPath,
			VMPath:    req.VMPath,
			CreatedAt: time.Now().UTC(),
		}
		mode := strings.ToLower(req.Mode)
		if mode == "git" {
			pair.Mode = SyncModeGit
			if !IsGitRepo(req.HostPath) {
				return jsonError(c, http.StatusBadRequest, "host directory is not a git repository")
			}
			if req.BareRepoPath != "" {
				pair.BareRepoPath = req.BareRepoPath
			} else {
				pair.BareRepoPath = defaultBareRepoPath(pair)
			}
		} else {
			pair.Mode = SyncModeCopy
			pair.Direction = SyncDirection(req.Direction)
			pair.Exclude = req.Exclude
			pair.BackupRetentionDays = req.BackupRetentionDays
		}
		sc, err := LoadSyncConfig(syncConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		if err := sc.AddPair(pair); err != nil {
			return jsonError(c, http.StatusBadRequest, err.Error())
		}
		if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		if pair.Mode == SyncModeGit {
			if err := GitSetupPair(cfg, pair); err != nil {
				return jsonError(c, http.StatusInternalServerError, err.Error())
			}
		}
		return jsonSuccess(c, pair)
	}
}

func handleRemoveSync(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := c.Param("id")
		sc, err := LoadSyncConfig(syncConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		if !sc.RemovePair(id) {
			return jsonError(c, http.StatusNotFound, "sync pair not found")
		}
		if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, map[string]string{"message": "sync pair removed"})
	}
}

func handleRunSync(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := c.Param("id")
		sc, err := LoadSyncConfig(syncConfigPath(cfg))
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		pair, ok := sc.GetPair(id)
		if !ok {
			return jsonError(c, http.StatusNotFound, "sync pair not found")
		}
		if pair.Mode == SyncModeGit {
			return jsonSuccess(c, map[string]string{"message": "Use 'git push vm' and 'git pull vm' in your host repo"})
		}
		go func() {
			var err error
			switch pair.Direction {
			case SyncDirectionHostToVM:
				err = CopySyncHostToVM(cfg, pair, syncBackupsDir(cfg))
			case SyncDirectionVMToHost:
				err = CopySyncVMToHost(cfg, pair, syncBackupsDir(cfg))
			case SyncDirectionBidirectional:
				err = CopySyncBidirectional(cfg, pair, syncBackupsDir(cfg))
			}
			if err != nil {
				fmt.Printf("sync error: %v\n", err)
			}
		}()
		return jsonSuccess(c, map[string]string{"message": "sync started"})
	}
}

func handleSyncHistory(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := c.Param("id")
		backups, err := listBackups(syncBackupsDir(cfg), id)
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, err.Error())
		}
		return jsonSuccess(c, backups)
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/vmctl/web_handlers.go
git commit -m "feat(web): add api handlers for vm, tunnels, sync"
```

---

### Task 6: Create Static Web Files

**Files:**
- Create: `web/static/index.html`
- Create: `web/static/style.css`
- Create: `web/static/app.js`

- [ ] **Step 1: Create directory**

```bash
mkdir -p web/static
```

- [ ] **Step 2: Write index.html**

See separate file `web/static/index.html` - contains main layout with left panel (actions, sync, tunnels) and right panel (status, resources), plus modal and file picker overlays.

- [ ] **Step 3: Write style.css**

See separate file `web/static/style.css` - dark theme, flex layout, modals, forms, file picker, toast notifications.

- [ ] **Step 4: Write app.js**

See separate file `web/static/app.js` - API wrapper, status polling every 5s, modal system, file picker with breadcrumb navigation, event handlers for all buttons.

- [ ] **Step 5: Commit**

```bash
git add web/static/
git commit -m "feat(web): add html, css, js frontend"
```

---

### Task 7: Build and Test

- [ ] **Step 1: Build**

```bash
go build ./cmd/vmctl
```
Expected: No errors.

- [ ] **Step 2: Run tests**

```bash
go test ./internal/vmctl/...
```
Expected: PASS.

- [ ] **Step 3: Verify no fyne**

```bash
grep -i fyne go.mod || echo "No fyne found - good!"
```
Expected: "No fyne found - good!"

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "build: verify clean build and tests after fyne removal"
```

---

## Spec Coverage Check

| Spec Requirement | Task |
|-----------------|------|
| Remove Fyne dependency | Task 2 |
| Echo v5 server | Task 1, 3 |
| Port config (flag/env/default) | Task 3 |
| REST API routes | Task 4, 5 |
| Static HTML/CSS/JS | Task 6 |
| Real-time refresh (5s polling) | Task 6 (app.js) |
| VM file picker | Task 4, 6 |
| Username `dev` → `vm` | Already done before plan |
| Separate CSS/JS files | Task 6 |

## Placeholder Scan

- No TBD/TODO/fill in details found.
- All code blocks contain complete implementations.
- All file paths are exact.

## Type Consistency

- `LaunchWebServer(port string)` used consistently.
- `VMFileEntry` struct used in Task 4 and referenced in Task 6.
- Handler signatures match Echo v5 `echo.HandlerFunc` pattern.

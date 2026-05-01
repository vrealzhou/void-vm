# Design: Migrate Fyne GUI to Web UI (Echo v5)

**Date:** 2026-05-01
**Topic:** fyne-to-web-migration

## Summary

Replace the existing Fyne-based desktop GUI in `vmctl` with a web-based UI served by a local Echo v5 HTTP server. The web UI uses separate HTML, CSS, and JS files and provides the same functionality as the current Fyne GUI: VM control (bootstrap/start/stop/destroy), status monitoring, SSH tunnel management, and folder sync management.

## Context

The `vmctl` tool currently uses `fyne.io/fyne/v2` for its control panel GUI (`gui.go`, `tunnel_gui.go`, `sync_gui.go`). Fyne pulls in many GUI-related dependencies (OpenGL, GLFW, etc.) and is heavier than necessary for a local management tool. A web-based UI is lighter, easier to style, and more accessible.

## Goals

1. Remove Fyne dependency entirely
2. Provide equivalent or better UX via a browser-based UI
3. Keep CSS and JS in separate files (not inline in HTML)
4. Support real-time status refresh
5. Allow configurable server port

## Non-Goals

- Multi-user support (still single-user local tool)
- Authentication (local-only server)
- Mobile-responsive design (desktop browser is sufficient)

## Architecture

```
┌─────────────────┐
│   Browser       │
│  (HTML/CSS/JS)  │
└────────┬────────┘
         │ HTTP
┌────────▼────────┐
│  Echo v5 Server │  ← Go code, listens on configurable port
│  (vmctl web)    │
└────────┬────────┘
         │ calls existing functions
┌────────▼────────┐
│  vmctl business │  ← InspectVM, Start, Stop, BootstrapSetup, etc.
│  logic (existing)│
└─────────────────┘
```

## File Structure

```
internal/vmctl/
├── web.go              # Echo server setup, route registration
├── web_handlers.go     # HTTP handler functions
├── web_static.go       # Static file embedding
├── gui.go              # REMOVED (Fyne code)
├── tunnel_gui.go       # REMOVED (Fyne code)
├── sync_gui.go         # REMOVED (Fyne code)
└── ...

web/static/
├── index.html          # Main page
├── style.css           # Styles
└── app.js              # Frontend logic
```

## API Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve index.html |
| GET | `/api/status` | VM status + guest metrics |
| POST | `/api/bootstrap` | Run bootstrap with preferences |
| POST | `/api/start` | Start VM |
| POST | `/api/stop` | Stop VM |
| POST | `/api/destroy` | Destroy VM |
| GET | `/api/tunnels` | List tunnels |
| POST | `/api/tunnels` | Add tunnel |
| DELETE | `/api/tunnels/:id` | Remove tunnel |
| POST | `/api/tunnels/:id/start` | Start tunnel |
| POST | `/api/tunnels/:id/stop` | Stop tunnel |
| GET | `/api/sync` | List sync pairs |
| POST | `/api/sync` | Add sync pair |
| DELETE | `/api/sync/:id` | Remove sync pair |
| POST | `/api/sync/:id/run` | Run sync |
| GET | `/api/sync/:id/history` | Backup history (copy mode) |
| GET | `/api/vm-files` | List files/dirs in VM via SSH (for file picker) |

## VM File Picker

A reusable file/directory picker for selecting paths inside the VM.

### API
- `GET /api/vm-files?path=<dir>` — List files and directories under the given path via SSH (`ls -la` or similar)
- Query param `root` optionally restricts navigation to a subtree root (e.g. `/home/vm/repos`)

### Behavior
- User is `vm`, home directory is `/home/vm`
- Default root for most selections: `/home/vm`
- Bare repo selections root: `/home/vm/repos`
- UI prevents navigating above the specified root (no `..` above root)
- Returns JSON: `[{"name":"...","isDir":true}, ...]`

## Username Change

All default usernames changed from `dev` to `vm`:
- `VM_SSH_USER` default: `vm`
- `VM_GUEST_USER` default: `vm`
- `VM_GUEST_PASSWORD` default: `vm`
- Default bare repo path: `/home/vm/repos/...`
- All test fixtures updated accordingly

### Frontend
- Modal dialog with breadcrumb navigation
- Click folder to enter, click file to select
- "Select" button confirms choice

## Port Configuration

Priority (highest to lowest):
1. `--port` CLI flag on the `web` subcommand
2. `VM_MANAGER_PORT` environment variable
3. Default: `8080`

## Frontend Design

- **Layout:** Left panel (actions, sync, tunnels) + right panel (VM status, resources)
- **Real-time refresh:** JS `setInterval` polls `/api/status` every 5 seconds
- **Notifications:** Toast messages for action success/failure
- **Modals:** Custom HTML modals for bootstrap preferences, add tunnel, add sync pair
- **Confirmations:** Browser `confirm()` for destructive actions (destroy, remove)

## Dependencies

**Remove:**
- `fyne.io/fyne/v2` and all indirect GUI dependencies

**Add:**
- `github.com/labstack/echo/v5`

## Error Handling

- All API handlers return JSON with consistent structure: `{"error": "..."}` or `{"success": true, ...}`
- Echo's recover middleware catches panics and returns 500
- Frontend displays error messages in toast notifications

## Testing Strategy

- Manual testing: start server, verify all CRUD operations via browser
- Verify port configuration works (flag, env var, default)
- Verify Fyne is fully removed (`go mod tidy` should not pull it back)

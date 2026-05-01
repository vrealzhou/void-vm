# SSH Port Tunnel Design for void-vm

**Date:** 2025-05-01
**Status:** Draft

## Overview

This document describes the SSH port tunneling feature for `void-vm`, which allows users to forward ports between the host machine and the Void Linux VM through SSH tunnels. Multiple tunnels can be active simultaneously, and all are managed through the vmctl GUI.

Two tunnel types are supported:

1. **Local Forward** — Forwards a port on the host to a port on the VM (host:local_port → vm:remote_port)
2. **Remote Forward** — Forwards a port on the VM to a port on the host (vm:remote_port → host:local_port)

## Goals

- Allow easy access to VM services from host (local forward)
- Allow VM services to access host services (remote forward)
- Support multiple simultaneous tunnels
- Manage tunnels entirely through GUI
- Zero external dependencies (uses system ssh command)

## Non-Goals

- Dynamic/SOCKS proxy (out of scope)
- Auto-reconnect with backoff (basic reconnect only)
- UDP forwarding (TCP only)

## Architecture

```
Host (macOS/Windows/Linux)                          VM (Void Linux)
┌─────────────────────────┐                        ┌─────────────────┐
│  localhost:8080         │◀── SSH -L 8080:... ───│  localhost:80   │
│  (browser, curl)        │                        │  (nginx, etc.)  │
└─────────────────────────┘                        └─────────────────┘
         ▲                                                    │
         │                                                    │
┌─────────────────────────┐                        ┌─────────────────┐
│  localhost:3000         │─── SSH -R 9090:... ───▶│  localhost:9090 │
│  (dev server)           │                        │  (app in VM)    │
└─────────────────────────┘                        └─────────────────┘
```

## Tunnel Types

### 1. Local Forward (`ssh -L`)

**Use case:** Access VM services from host

**Example:**
- VM runs a web server on port 80
- Host accesses it via `localhost:8080`
- SSH command: `ssh -N -L 8080:localhost:80 dev@192.168.64.10`

**Direction:** host → VM

### 2. Remote Forward (`ssh -R`)

**Use case:** Let VM services access host services

**Example:**
- Host runs a dev server on port 3000
- VM accesses it via `localhost:9090`
- SSH command: `ssh -N -R 9090:localhost:3000 dev@192.168.64.10`

**Direction:** VM → host

## Implementation

### Technology

- **System SSH command** — Uses existing `ssh` binary via `os/exec`
- **Background processes** — Each tunnel runs as a separate `ssh -N` process
- **PID tracking** — Store PID in `.vm/tunnels/{tunnel-id}.pid`
- **Process lifecycle** — Start/stop via GUI, cleanup on VM stop

### SSH Command Details

```bash
# Local forward
ssh -N \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -o ExitOnForwardFailure=yes \
  -L LOCAL_PORT:REMOTE_HOST:REMOTE_PORT \
  dev@192.168.64.10

# Remote forward
ssh -N \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -o ExitOnForwardFailure=yes \
  -R REMOTE_PORT:LOCAL_HOST:LOCAL_PORT \
  dev@192.168.64.10
```

Options:
- `-N` — Don't execute remote command (just port forwarding)
- `-o ServerAliveInterval=30` — Keep connection alive
- `-o ExitOnForwardFailure=yes` — Exit if port forwarding fails

### Config Storage

Tunnels stored in `.vmctl.tunnels` (JSON, repo-root):

```json
{
  "version": 1,
  "tunnels": [
    {
      "id": "web-server",
      "name": "Web Server",
      "type": "local",
      "local_port": 8080,
      "remote_host": "localhost",
      "remote_port": 80,
      "enabled": true,
      "created_at": "2025-05-01T10:00:00Z"
    },
    {
      "id": "api-dev",
      "name": "API Dev",
      "type": "local",
      "local_port": 3001,
      "remote_host": "localhost",
      "remote_port": 3000,
      "enabled": false,
      "created_at": "2025-05-01T10:30:00Z"
    },
    {
      "id": "host-dev",
      "name": "Host Dev Server",
      "type": "remote",
      "remote_port": 9090,
      "local_host": "localhost",
      "local_port": 3000,
      "enabled": true,
      "created_at": "2025-05-01T11:00:00Z"
    }
  ]
}
```

### File Structure

```
internal/vmctl/
  tunnel_config.go     # Tunnel config types, load/save
  tunnel_manager.go    # Tunnel lifecycle (start/stop/status)
  tunnel_gui.go        # GUI: tunnel panel, dialogs
  tunnel_cli.go        # CLI: tunnel subcommands
```

### New Files

- `internal/vmctl/tunnel_config.go` — Config types and storage
- `internal/vmctl/tunnel_manager.go` — SSH process management
- `internal/vmctl/tunnel_gui.go` — GUI widgets
- `internal/vmctl/tunnel_cli.go` — CLI commands

### Modified Files

- `internal/vmctl/cobra.go` — Add tunnel subcommand
- `internal/vmctl/gui.go` — Add tunnel panel to main window
- `internal/vmctl/vm.go` — Stop all tunnels when VM stops

## GUI Design

### Main Window Addition

Add a "Port Tunnels" section below the existing action buttons:

```
┌─────────────────────────────────────────┐
│  Actions                                │
│  [Bootstrap] [Start] [Stop] [Destroy]   │
├─────────────────────────────────────────┤
│  Port Tunnels                           │
├─────────────────────────────────────────┤
│  🟢 web-server  host:8080 → vm:80       │
│     [Local]  [Stop]  [🗑️]               │
├─────────────────────────────────────────┤
│  🔴 api-dev     host:3001 → vm:3000     │
│     [Local]  [Start]  [🗑️]              │
├─────────────────────────────────────────┤
│  🟢 host-dev    vm:9090 → host:3000     │
│     [Remote]  [Stop]  [🗑️]              │
├─────────────────────────────────────────┤
│  [+ Add Tunnel]                         │
└─────────────────────────────────────────┘
```

Status indicators:
- 🟢 — Tunnel is active (process running)
- 🔴 — Tunnel is stopped
- 🟡 — Tunnel is starting/stopping

### Add Tunnel Dialog

```
┌─────────────────────────────────────────┐
│  Add Port Tunnel                        │
├─────────────────────────────────────────┤
│  Name: [web-server        ]             │
│                                         │
│  Type: [Local Forward ▼]                │
│                                         │
│  ── Local Forward ──                    │
│  Host Port:     [8080      ]            │
│  VM Host:       [localhost ]            │
│  VM Port:       [80        ]            │
│                                         │
│  ── Remote Forward ──                   │
│  VM Port:       [9090      ]            │
│  Host Host:     [localhost ]            │
│  Host Port:     [3000      ]            │
│                                         │
│  [Auto-start when VM runs] ☑            │
│                                         │
│        [Cancel]    [Add Tunnel]         │
└─────────────────────────────────────────┘
```

Fields:
- **Name** — Unique identifier for the tunnel
- **Type** — Local Forward or Remote Forward
- **Host Port** — Port on host machine (local forward) or port to forward to (remote forward)
- **VM Host** — Hostname inside VM (usually localhost)
- **VM Port** — Port inside VM
- **Auto-start** — Automatically start tunnel when VM starts

## CLI Commands

```bash
# List all tunnels
go run ./cmd/vmctl tunnel list

# Add a local forward tunnel
go run ./cmd/vmctl tunnel add \
  --name web \
  --type local \
  --local-port 8080 \
  --remote-port 80

# Add a remote forward tunnel
go run ./cmd/vmctl tunnel add \
  --name dev \
  --type remote \
  --remote-port 9090 \
  --local-port 3000

# Start a tunnel
go run ./cmd/vmctl tunnel start web

# Stop a tunnel
go run ./cmd/vmctl tunnel stop web

# Remove a tunnel
go run ./cmd/vmctl tunnel remove web

# Start all enabled tunnels
go run ./cmd/vmctl tunnel start-all

# Stop all tunnels
go run ./cmd/vmctl tunnel stop-all
```

## Lifecycle

### Starting a Tunnel

1. Check if VM is running (required)
2. Check if local port is available
3. Build SSH command with appropriate `-L` or `-R` flag
4. Start SSH process in background
5. Write PID to `.vm/tunnels/{tunnel-id}.pid`
6. Verify tunnel is working (optional: try connecting to local port)

### Stopping a Tunnel

1. Read PID from `.vm/tunnels/{tunnel-id}.pid`
2. Send SIGTERM to process
3. Wait up to 5 seconds for graceful shutdown
4. Send SIGKILL if still running
5. Remove PID file

### VM Stop Handling

When VM is stopped:
1. Stop all active tunnels
2. Clean up PID files

### VM Start Handling

When VM starts:
1. Check for tunnels with `auto_start: true`
2. Start those tunnels (after SSH is available)

## Error Handling

| Scenario | Behavior |
|----------|----------|
| VM not running | Show error: "VM is not running. Start it first." |
| Local port in use | Show error with port number, suggest alternative |
| SSH connection fails | Show error details, mark tunnel as failed |
| Tunnel process dies | Detect on next status check, show as stopped |
| Port forward fails | `ExitOnForwardFailure=yes` causes ssh to exit, detect and show error |

## Security Considerations

1. **Localhost only** — By default, tunnels bind to localhost (127.0.0.1) only
2. **No gateway ports** — Unless explicitly requested, tunnels are not accessible from other machines on the network
3. **SSH security** — Uses existing SSH key authentication
4. **Port validation** — Prevent forwarding privileged ports (<1024) without elevated permissions

## Testing Strategy

1. **Unit tests:**
   - Config load/save
   - SSH command generation
   - PID file management

2. **Integration tests:**
   - Start/stop tunnel
   - Multiple tunnels
   - Port conflict detection

3. **Manual tests:**
   - Local forward: host browser → VM web server
   - Remote forward: VM curl → host dev server
   - Multiple simultaneous tunnels
   - VM stop/start with auto-start tunnels

## Future Enhancements (Out of Scope)

- Dynamic/SOCKS proxy (`ssh -D`)
- Auto-reconnect with exponential backoff
- UDP forwarding
- Bandwidth limiting per tunnel
- Tunnel usage statistics

## Decisions

1. ✅ Use system SSH command (not Go crypto/ssh library)
2. ✅ Support both local and remote forward
3. ✅ Multiple simultaneous tunnels
4. ✅ Auto-start option per tunnel
5. ✅ GUI and CLI both supported

## Spec Self-Review

### Placeholder Scan
- No TBD or TODO items found
- All sections are complete

### Internal Consistency
- Tunnel types (local/remote) are clearly defined
- SSH command options are consistent
- GUI and CLI functionality match

### Scope Check
- Focused on SSH port forwarding only
- No feature creep
- Appropriate for a single implementation plan

### Ambiguity Check
- Local vs remote forward is clearly distinguished
- Port binding defaults to localhost
- Process lifecycle is well-defined

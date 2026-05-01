# File Sync Design for void-vm

**Date:** 2025-05-01
**Status:** Draft

## Overview

This document describes the file synchronization feature for `void-vm`, which allows users to sync folders between the host machine (macOS/Windows) and the Void Linux VM. Two sync modes are supported:

1. **Git Mode** — For git repositories. Sets up a bare repo in the VM and lets users sync via standard git commands.
2. **Copy Mode** — For non-git folders. Uses rsync (via the `gokrazy/rsync` pure-Go library) to sync files, with host-side versioning for rollback.

Both modes are managed entirely through the vmctl GUI.

## Goals

- Provide two complementary sync strategies for different use cases
- Require zero external dependencies on the host (pure Go)
- Allow full management through the Fyne GUI
- Support cross-platform hosts (macOS, Windows, Linux)
- Enable version control / rollback for Copy mode

## Non-Goals

- Real-time/auto-sync (out of scope; manual trigger only)
- Cloud storage integration
- Sync between two VMs

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Host (macOS/Windows/Linux)            │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────────┐ │
│  │  vmctl GUI  │───▶│  Sync Manager │───▶│ gokrazy/rsync   │ │
│  │             │    │  (Go code)    │    │  client (copy)  │ │
│  └─────────────┘    └──────────────┘    └─────────────────┘ │
│                              │                               │
│                              ▼                               │
│                        ┌──────────┐                          │
│                        │   SSH    │                          │
│                        └──────────┘                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      VM (Void Linux)                         │
│  ┌──────────────┐    ┌─────────────────────────────────────┐ │
│  │  gokr-rsync   │◀───│  SSH (starts gokr-rsync on demand)  │ │
│  │  server       │    └─────────────────────────────────────┘ │
│  └──────────────┘                                            │
│         │                                                    │
│         ▼                                                    │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │ Git bare repo │    │  Copy mode   │    │  ~/repos/    │   │
│  │  (git mode)   │    │  (rsync)     │    │  ~/sync/     │   │
│  └──────────────┘    └──────────────┘    └──────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Sync Modes

### 1. Git Mode

**Use case:** Code projects that are already (or should be) git repositories.

**Workflow:**

1. User clicks "Add Sync Pair" in GUI
2. User selects:
   - Host source directory (must be a git repo or will be `git init`)
   - VM target directory (where to `git clone`)
   - Bare repo path in VM (e.g., `~/repos/myproject/repo.git`)
3. vmctl automatically:
   - SSH into VM
   - Create parent dirs: `mkdir -p ~/repos/myproject`
   - Init bare repo: `git init --bare ~/repos/myproject/repo.git`
   - Add `vm` remote on host: `git remote add vm ssh://dev@192.168.64.10/~/repos/myproject/repo.git`
   - Push all branches: `git push vm --all`
   - Push tags: `git push vm --tags`
   - Clone into VM target dir: `git clone ~/repos/myproject/repo.git ~/work/myproject`
4. **Subsequent syncs:** User runs git commands manually (push/pull/fetch)

**Key points:**
- After initial setup, sync is entirely user-driven via git
- No background processes
- Works with existing git workflows
- Host can be Windows (git for Windows required, but most devs have it)

### 2. Copy Mode

**Use case:** Non-code folders (documents, configs, assets) that need file-level sync with versioning.

**Technology:** `gokrazy/rsync` — pure Go implementation of rsync protocol 27

**Workflow:**

1. User clicks "Add Sync Pair" in GUI
2. User selects:
   - Host source directory
   - VM target directory
   - Sync direction (Host→VM, VM→Host, or Bidirectional)
   - Exclude patterns (optional, e.g., `*.tmp`, `.DS_Store`)
   - Exclude-from file (optional, path to a `.gitignore`-style file)
3. vmctl:
   - Ensures `gokr-rsync` binary exists on VM (install if missing)
   - Stores sync pair config
4. User clicks "Sync" button
5. vmctl:
   - Computes file diffs using rsync protocol
   - Transfers changed files via SSH
   - For Host→VM or bidirectional: backs up overwritten files on host to `.vm/sync-backups/{pair-id}/{timestamp}/`
   - Shows progress in GUI

**Version control / Rollback:**

- Before overwriting files on host, copy old version to `.vm/sync-backups/{pair-id}/{timestamp}/`
- GUI provides "History" button to browse backups
- User can select a timestamp and restore files from that backup
- Backups are stored on host only (VM modifications are synced to host, then host keeps history)
- **Retention policy:** Configurable per sync pair, default 7 days. Backups older than retention period are automatically cleaned up on next sync.

**Key points:**
- Manual trigger only (no auto-sync)
- Incremental transfer (only changed blocks)
- Host-side backup for rollback
- Pure Go, no external dependencies

## VM Side Setup

### gokr-rsync Installation

Option A: Pre-compile and upload (recommended)
```bash
# On host (cross-compile for Linux arm64)
GOOS=linux GOARCH=arm64 go build -o gokr-rsync \
  github.com/gokrazy/rsync/cmd/gokr-rsync

# Upload to VM via SSH
scp gokr-rsync dev@192.168.64.10:/usr/local/bin/
```

Option B: Install via bootstrap
Add to `scripts/guest-bootstrap.sh`:
```bash
# Install gokr-rsync
xbps-install -y go
su - dev -c "go install github.com/gokrazy/rsync/cmd/gokr-rsync@latest"
```

### Git Mode VM Setup

No special setup needed beyond standard git (already installed in bootstrap).

## Host Side Implementation

### New Files

```
internal/vmctl/
  sync.go          # Core sync logic (git + copy modes)
  sync_git.go      # Git mode implementation
  sync_copy.go     # Copy mode implementation (gokrazy/rsync)
  sync_config.go   # Sync pair config loading/saving
  sync_gui.go      # GUI dialogs and widgets for sync
```

### Dependencies

Add to `go.mod`:
```
github.com/gokrazy/rsync v0.3.3
```

### Config Storage

Sync pairs stored in `.vmctl.sync` (JSON, repo-root):
```json
{
  "version": 1,
  "sync_pairs": [
    {
      "id": "myapp",
      "mode": "git",
      "host_path": "/Users/dev/projects/myapp",
      "vm_path": "/home/dev/work/myapp",
      "bare_repo_path": "/home/dev/repos/myapp/repo.git",
      "created_at": "2025-05-01T10:00:00Z"
    },
    {
      "id": "docs",
      "mode": "copy",
      "host_path": "/Users/dev/docs",
      "vm_path": "/home/dev/shared/docs",
      "direction": "bidirectional",
      "exclude": [".tmp", "*.log", ".DS_Store"],
      "exclude_from": "/Users/dev/.vmctl-exclude-docs",
      "backup_retention_days": 7,
      "created_at": "2025-05-01T10:30:00Z"
    }
  ]
}
```

## GUI Design

### Main Window Addition

Add a "Sync Folders" section below the existing action buttons:

```
┌─────────────────────────────────────────┐
│  Actions                                │
│  [Bootstrap] [Start] [Stop] [Destroy]   │
├─────────────────────────────────────────┤
│  Sync Folders                           │
├─────────────────────────────────────────┤
│  📁 myapp  ~/projects/myapp ↔ ~/work/   │
│     [Git]  [▶ Sync]  [⚙ Settings] [🗑️]  │
├─────────────────────────────────────────┤
│  📁 docs   ~/docs ↔ ~/shared/docs       │
│     [Copy] [▶ Sync]  [📜 History] [🗑️]  │
├─────────────────────────────────────────┤
│  [+ Add Folder Pair]                    │
└─────────────────────────────────────────┘
```

### Add Folder Pair Dialog

```
┌─────────────────────────────────────────┐
│  Add Folder Pair                        │
├─────────────────────────────────────────┤
│  Mode: [Git ▼] [Copy]                   │
│                                         │
│  Host Directory:  [Browse...]           │
│  /Users/dev/projects/myapp              │
│                                         │
│  VM Directory:    [Browse...]           │
│  /home/dev/work/myapp                   │
│                                         │
│  ── Git Mode Only ──                    │
│  Bare Repo Path:  [~/repos/myapp/repo]  │
│                                         │
│  ── Copy Mode Only ──                   │
│  Direction: [Bidirectional ▼]           │
│  Exclude:   [*.tmp, .DS_Store]          │
│  Exclude File: [~/.vmctl-exclude-docs]  │
│  Retention:  [7 days ▼]                 │
│                                         │
│        [Cancel]    [Add Pair]           │
└─────────────────────────────────────────┘
```

- **Host Directory**: File picker (Fyne's `dialog.ShowFolderOpen`)
- **VM Directory**: Text input (validated via SSH on save)
- **Bare Repo Path** (Git mode): Text input, default `~/repos/{basename}/repo.git`
- **Direction** (Copy mode): Host→VM, VM→Host, Bidirectional
- **Exclude** (Copy mode): Comma-separated glob patterns
- **Exclude From** (Copy mode): Path to a `.gitignore`-style exclude file (one pattern per line)

### Sync Progress Dialog

```
┌─────────────────────────────────────────┐
│  Syncing: docs                          │
├─────────────────────────────────────────┤
│  Scanning files...                      │
│  [████████░░░░░░░░░░] 45%               │
│                                         │
│  src/main.go        [=====>    ] 65%    │
│  config.yml         [Pending]           │
│                                         │
│  Transferred: 12.5 MB / 45.2 MB         │
│  Files: 23 / 156                        │
│                                         │
│              [Cancel]                   │
└─────────────────────────────────────────┘
```

### History Dialog (Copy Mode)

```
┌─────────────────────────────────────────┐
│  Backup History: docs                   │
├─────────────────────────────────────────┤
│  2025-05-01 14:30  (12 files, 2.3 MB)  │
│  2025-05-01 12:15  (8 files, 1.1 MB)   │
│  2025-05-01 10:00  (3 files, 0.5 MB)   │
│                                         │
│  [View Files]  [Restore Selected]       │
└─────────────────────────────────────────┘
```

## CLI Commands

Add new subcommands:

```bash
# List configured sync pairs
go run ./cmd/vmctl sync list

# Add a git sync pair
go run ./cmd/vmctl sync add-git \
  --host-dir ~/projects/myapp \
  --vm-dir ~/work/myapp \
  --bare-repo ~/repos/myapp/repo.git

# Add a copy sync pair
go run ./cmd/vmctl sync add-copy \
  --host-dir ~/docs \
  --vm-dir ~/shared/docs \
  --direction bidirectional \
  --exclude "*.tmp,.DS_Store"

# Sync a specific pair
go run ./cmd/vmctl sync run docs

# Remove a sync pair
go run ./cmd/vmctl sync remove docs

# Show backup history
go run ./cmd/vmctl sync history docs

# Restore from backup
go run ./cmd/vmctl sync restore docs --timestamp 2025-05-01T14:30:00Z
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| VM not running | Show error: "VM is not running. Start it first." |
| Host dir not found | Validate on add, reject with clear message |
| VM dir not accessible | SSH test on add, show error dialog |
| Git dir not a repo | Offer to `git init` or reject |
| rsync connection fails | Retry once, then show error with details |
| Sync conflict (copy) | For bidirectional: newer file wins, backup old version |
| Disk full (backups) | Warn user, offer to clean old backups |

## Security Considerations

1. **SSH only** — All communication goes through existing SSH tunnel
2. **No exposed ports** — gokr-rsync runs on-demand via SSH, no daemon listening
3. **Host backups** — Backups stay on host, not sent over network
4. **Path validation** — Prevent path traversal in VM directory paths

## Testing Strategy

1. **Unit tests:**
   - Config load/save
   - Path validation
   - Exclude pattern matching
   - Backup/restore logic

2. **Integration tests:**
   - Git mode: full setup + push + clone flow
   - Copy mode: sync + verify + rollback
   - GUI: dialog interactions

3. **Manual tests:**
   - macOS host → VM
   - Windows host → VM (via WSL or native)
   - Large file sync (>1GB)
   - Many small files (>1000)

## Future Enhancements (Out of Scope)

- Auto-sync on file change (fsnotify watcher)
- Sync scheduling (cron-like)
- Cloud backup integration (S3, etc.)
- Sync statistics and bandwidth limiting
- Compression options for copy mode

## Spec Self-Review

### Placeholder Scan
- No TBD or TODO items found
- All sections are complete

### Internal Consistency
- Git mode workflow matches user's requirement (manual sync after setup)
- Copy mode uses gokrazy/rsync which is pure Go and cross-platform
- GUI design is consistent with existing Fyne-based GUI
- CLI commands mirror GUI functionality

### Scope Check
- Focused on two sync modes only
- No feature creep (auto-sync, cloud, etc. are explicitly out of scope)
- Appropriate for a single implementation plan

### Ambiguity Check
- Git mode: "subsequent syncs are user-driven via git commands" is clear
- Copy mode: "manual trigger only" is explicit
- Backup location is specified: `.vm/sync-backups/{pair-id}/{timestamp}/`

## Decisions

1. ✅ Copy mode supports `.gitignore`-style exclude files via `exclude_from` field
2. ✅ Backup retention is configurable per sync pair, default **7 days**
3. ❌ VM→different host directory not supported (bidirectional within same pair only)

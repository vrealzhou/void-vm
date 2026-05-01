package vmctl

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func makeSyncPanel(cfg Config, w fyne.Window, refresh func()) fyne.CanvasObject {
	listContainer := container.NewVBox()

	reload := func() {
		listContainer.Objects = nil
		sc, err := LoadSyncConfig(syncConfigPath(cfg))
		if err != nil {
			listContainer.Add(widget.NewLabel("Error loading sync config: " + err.Error()))
			listContainer.Refresh()
			return
		}

		if len(sc.Pairs) == 0 {
			listContainer.Add(widget.NewLabel("No sync pairs configured."))
		} else {
			for _, pair := range sc.Pairs {
				listContainer.Add(makeSyncPairRow(cfg, pair, w, refresh))
			}
		}
		listContainer.Refresh()
	}

	addButton := widget.NewButtonWithIcon("Add Folder Pair", theme.ContentAddIcon(), func() {
		showAddPairDialog(cfg, w, func() {
			reload()
			if refresh != nil {
				refresh()
			}
		})
	})

	panel := container.NewVBox(
		widget.NewLabelWithStyle("Sync Folders", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		listContainer,
		addButton,
	)

	reload()
	return panel
}

func makeSyncPairRow(cfg Config, pair SyncPair, w fyne.Window, refresh func()) fyne.CanvasObject {
	modeLabel := widget.NewButton(string(pair.Mode), nil)
	modeLabel.Disable()

	syncButton := widget.NewButtonWithIcon("Sync", theme.MediaPlayIcon(), func() {
		runSyncPair(cfg, pair, w)
	})

	removeButton := widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
		confirmRemovePair(cfg, pair.ID, w, refresh)
	})

	var buttons []fyne.CanvasObject
	buttons = append(buttons, modeLabel, syncButton, removeButton)

	if pair.Mode == SyncModeCopy {
		historyButton := widget.NewButtonWithIcon("History", theme.DocumentIcon(), func() {
			showHistoryDialog(cfg, pair, w)
		})
		buttons = append(buttons, historyButton)
	}

	info := fmt.Sprintf("%s  %s ↔ %s", pair.ID, pair.HostPath, pair.VMPath)
	if pair.Mode == SyncModeCopy {
		info = fmt.Sprintf("%s  %s  %s ↔ %s", pair.ID, pair.Direction, pair.HostPath, pair.VMPath)
	}

	return container.NewVBox(
		widget.NewLabel(info),
		container.NewHBox(buttons...),
		widget.NewSeparator(),
	)
}

func showAddPairDialog(cfg Config, w fyne.Window, refresh func()) {
	modeSelect := widget.NewSelect([]string{"Git", "Copy"}, nil)
	modeSelect.SetSelected("Git")

	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("Host directory path")

	vmEntry := widget.NewEntry()
	vmEntry.SetPlaceHolder("VM directory path")

	bareRepoEntry := widget.NewEntry()
	bareRepoEntry.SetPlaceHolder("Bare repo path on VM (optional)")

	directionSelect := widget.NewSelect([]string{
		string(SyncDirectionHostToVM),
		string(SyncDirectionVMToHost),
		string(SyncDirectionBidirectional),
	}, nil)
	directionSelect.SetSelected(string(SyncDirectionHostToVM))

	excludeEntry := widget.NewEntry()
	excludeEntry.SetPlaceHolder("Comma-separated exclude patterns (optional)")

	retentionEntry := widget.NewEntry()
	retentionEntry.SetPlaceHolder("Backup retention days (default: 7)")

	var items []*widget.FormItem
	items = append(items,
		widget.NewFormItem("Mode", modeSelect),
		widget.NewFormItem("Host directory", hostEntry),
		widget.NewFormItem("VM directory", vmEntry),
		widget.NewFormItem("Bare repo path", bareRepoEntry),
		widget.NewFormItem("Direction", directionSelect),
		widget.NewFormItem("Exclude patterns", excludeEntry),
		widget.NewFormItem("Retention days", retentionEntry),
	)

	dialog.ShowForm("Add Sync Pair", "Add", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}

		hostDir := hostEntry.Text
		vmDir := vmEntry.Text

		if hostDir == "" || vmDir == "" {
			dialog.ShowError(fmt.Errorf("host directory and VM directory are required"), w)
			return
		}

		pair := SyncPair{
			ID:        filepath.Base(hostDir),
			HostPath:  hostDir,
			VMPath:    vmDir,
			CreatedAt: time.Now().UTC(),
		}

		mode := strings.ToLower(modeSelect.Selected)
		if mode == "git" {
			pair.Mode = SyncModeGit
			if !IsGitRepo(hostDir) {
				dialog.ShowError(fmt.Errorf("host directory %q is not a git repository", hostDir), w)
				return
			}
			if bareRepoEntry.Text != "" {
				pair.BareRepoPath = bareRepoEntry.Text
			} else {
				pair.BareRepoPath = defaultBareRepoPath(cfg, pair)
			}
		} else {
			pair.Mode = SyncModeCopy
			pair.Direction = SyncDirection(directionSelect.Selected)
			if excludeEntry.Text != "" {
				pair.Exclude = strings.Split(excludeEntry.Text, ",")
			}
			if retentionEntry.Text != "" {
				var days int
				if _, err := fmt.Sscanf(retentionEntry.Text, "%d", &days); err == nil {
					pair.BackupRetentionDays = days
				}
			}
		}

		sc, err := LoadSyncConfig(syncConfigPath(cfg))
		if err != nil {
			dialog.ShowError(fmt.Errorf("load sync config: %w", err), w)
			return
		}

		if err := sc.AddPair(pair); err != nil {
			dialog.ShowError(err, w)
			return
		}

		if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
			dialog.ShowError(fmt.Errorf("save sync config: %w", err), w)
			return
		}

		if pair.Mode == SyncModeGit {
			if err := GitSetupPair(cfg, pair); err != nil {
				dialog.ShowError(fmt.Errorf("git setup: %w", err), w)
				return
			}
		}

		if refresh != nil {
			refresh()
		}
	}, w)
}

func runSyncPair(cfg Config, pair SyncPair, w fyne.Window) {
	switch pair.Mode {
	case SyncModeGit:
		dialog.ShowInformation("Git Sync", "Use 'git push vm' and 'git pull vm' in your host repo.", w)
	case SyncModeCopy:
		go func() {
			var err error
			switch pair.Direction {
			case SyncDirectionHostToVM:
				err = CopySyncHostToVM(cfg, pair, syncBackupsDir(cfg))
			case SyncDirectionVMToHost:
				err = CopySyncVMToHost(cfg, pair, syncBackupsDir(cfg))
			case SyncDirectionBidirectional:
				err = CopySyncBidirectional(cfg, pair, syncBackupsDir(cfg))
			default:
				err = fmt.Errorf("unknown direction: %s", pair.Direction)
			}
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("sync failed: %w", err), w)
				} else {
					dialog.ShowInformation("Sync Complete", fmt.Sprintf("Sync completed for %q", pair.ID), w)
				}
			})
		}()
	default:
		dialog.ShowError(fmt.Errorf("unknown sync mode: %s", pair.Mode), w)
	}
}

func confirmRemovePair(cfg Config, pairID string, w fyne.Window, refresh func()) {
	confirm := dialog.NewConfirm(
		"Remove Sync Pair",
		fmt.Sprintf("Remove sync pair %q?", pairID),
		func(ok bool) {
			if !ok {
				return
			}
			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				dialog.ShowError(fmt.Errorf("load sync config: %w", err), w)
				return
			}
			if !sc.RemovePair(pairID) {
				dialog.ShowError(fmt.Errorf("sync pair %q not found", pairID), w)
				return
			}
			if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
				dialog.ShowError(fmt.Errorf("save sync config: %w", err), w)
				return
			}
			if refresh != nil {
				refresh()
			}
		},
		w,
	)
	confirm.SetConfirmText("Remove")
	confirm.SetConfirmImportance(widget.DangerImportance)
	confirm.Show()
}

func showHistoryDialog(cfg Config, pair SyncPair, w fyne.Window) {
	backups, err := listBackups(syncBackupsDir(cfg), pair.ID)
	if err != nil {
		dialog.ShowError(fmt.Errorf("list backups: %w", err), w)
		return
	}
	if len(backups) == 0 {
		dialog.ShowInformation("Backup History", "No backups found.", w)
		return
	}
	var lines []string
	for _, ts := range backups {
		lines = append(lines, ts.Format(time.RFC3339))
	}
	dialog.ShowInformation("Backup History for "+pair.ID, strings.Join(lines, "\n"), w)
}

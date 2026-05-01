package vmctl

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func makeTunnelPanel(cfg Config, w fyne.Window, refresh func()) fyne.CanvasObject {
	listContainer := container.NewVBox()

	var reload func()
	reload = func() {
		listContainer.Objects = nil
		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			listContainer.Add(widget.NewLabel("Error loading tunnel config: " + err.Error()))
			listContainer.Refresh()
			return
		}

		if len(tc.Tunnels) == 0 {
			listContainer.Add(widget.NewLabel("No tunnels configured."))
		} else {
			for _, tunnel := range tc.Tunnels {
				listContainer.Add(makeTunnelRow(cfg, tunnel, w, reload))
			}
		}
		listContainer.Refresh()
	}

	addButton := widget.NewButtonWithIcon("Add Tunnel", theme.ContentAddIcon(), func() {
		showAddTunnelDialog(cfg, w, func() {
			reload()
			if refresh != nil {
				refresh()
			}
		})
	})

	panel := container.NewVBox(
		widget.NewLabelWithStyle("Port Tunnels", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		listContainer,
		addButton,
	)

	reload()
	return panel
}

func makeTunnelRow(cfg Config, tunnel Tunnel, w fyne.Window, refresh func()) fyne.CanvasObject {
	isRunning := IsTunnelRunning(cfg, tunnel)

	statusIcon := "\U0001F534" // red circle
	actionLabel := "Start"
	actionIcon := theme.MediaPlayIcon()
	if isRunning {
		statusIcon = "\U0001F7E2" // green circle
		actionLabel = "Stop"
		actionIcon = theme.MediaStopIcon()
	}

	remoteHost := tunnel.RemoteHost
	if remoteHost == "" {
		remoteHost = "localhost"
	}

	var mapping string
	if tunnel.Type == TunnelTypeLocal {
		mapping = fmt.Sprintf("host:%d → %s:%d", tunnel.LocalPort, remoteHost, tunnel.RemotePort)
	} else {
		mapping = fmt.Sprintf("vm:%d → host:%d", tunnel.RemotePort, tunnel.LocalPort)
	}

	info := fmt.Sprintf("%s %s     %s", statusIcon, tunnel.Name, mapping)

	typeLabel := widget.NewButton(string(tunnel.Type), nil)
	typeLabel.Disable()

	actionButton := widget.NewButtonWithIcon(actionLabel, actionIcon, func() {
		runTunnelAction(cfg, tunnel, w, refresh)
	})

	removeButton := widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
		confirmRemoveTunnel(cfg, tunnel.ID, w, refresh)
	})

	return container.NewVBox(
		widget.NewLabel(info),
		container.NewHBox(typeLabel, actionButton, removeButton),
		widget.NewSeparator(),
	)
}

func showAddTunnelDialog(cfg Config, w fyne.Window, refresh func()) {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Tunnel name")

	typeSelect := widget.NewSelect([]string{"Local Forward", "Remote Forward"}, nil)
	typeSelect.SetSelected("Local Forward")

	localPortEntry := widget.NewEntry()
	localPortEntry.SetPlaceHolder("Local port number")

	remotePortEntry := widget.NewEntry()
	remotePortEntry.SetPlaceHolder("Remote port number")

	remoteHostEntry := widget.NewEntry()
	remoteHostEntry.SetPlaceHolder("Remote host (default: localhost)")

	autoStartCheck := widget.NewCheck("Auto-start when VM runs", nil)

	items := []*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Type", typeSelect),
		widget.NewFormItem("Local port", localPortEntry),
		widget.NewFormItem("Remote port", remotePortEntry),
		widget.NewFormItem("Remote host", remoteHostEntry),
		widget.NewFormItem("Auto-start", autoStartCheck),
	}

	dialog.ShowForm("Add Tunnel", "Add", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}

		name := nameEntry.Text
		if name == "" {
			dialog.ShowError(fmt.Errorf("name is required"), w)
			return
		}

		localPort, err := strconv.Atoi(localPortEntry.Text)
		if err != nil || localPort <= 0 || localPort > 65535 {
			dialog.ShowError(fmt.Errorf("local port must be a valid port number (1-65535)"), w)
			return
		}

		remotePort, err := strconv.Atoi(remotePortEntry.Text)
		if err != nil || remotePort <= 0 || remotePort > 65535 {
			dialog.ShowError(fmt.Errorf("remote port must be a valid port number (1-65535)"), w)
			return
		}

		var tunnelType TunnelType
		if typeSelect.Selected == "Local Forward" {
			tunnelType = TunnelTypeLocal
		} else {
			tunnelType = TunnelTypeRemote
		}

		remoteHost := remoteHostEntry.Text
		if remoteHost == "" {
			remoteHost = "localhost"
		}

		tunnel := Tunnel{
			ID:         name,
			Name:       name,
			Type:       tunnelType,
			LocalPort:  localPort,
			RemoteHost: remoteHost,
			RemotePort: remotePort,
			Enabled:    true,
			AutoStart:  autoStartCheck.Checked,
			CreatedAt:  time.Now().UTC(),
		}

		tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
		if err != nil {
			dialog.ShowError(fmt.Errorf("load tunnel config: %w", err), w)
			return
		}

		if err := tc.AddTunnel(tunnel); err != nil {
			dialog.ShowError(err, w)
			return
		}

		if err := SaveTunnelConfig(tunnelConfigPath(cfg), tc); err != nil {
			dialog.ShowError(fmt.Errorf("save tunnel config: %w", err), w)
			return
		}

		if refresh != nil {
			refresh()
		}
	}, w)
}

func runTunnelAction(cfg Config, tunnel Tunnel, w fyne.Window, refresh func()) {
	isRunning := IsTunnelRunning(cfg, tunnel)

	if isRunning {
		go func() {
			err := StopTunnel(cfg, tunnel)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("stop tunnel: %w", err), w)
				} else {
					dialog.ShowInformation("Tunnel Stopped", fmt.Sprintf("Tunnel %q stopped.", tunnel.Name), w)
				}
				if refresh != nil {
					refresh()
				}
			})
		}()
	} else {
		go func() {
			status, err := InspectVM(cfg)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("inspect VM: %w", err), w)
				})
				return
			}
			if !status.Running {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("VM is not running"), w)
				})
				return
			}

			err = StartTunnel(cfg, tunnel)
			fyne.Do(func() {
				if err != nil {
					dialog.ShowError(fmt.Errorf("start tunnel: %w", err), w)
				} else {
					dialog.ShowInformation("Tunnel Started", fmt.Sprintf("Tunnel %q started.", tunnel.Name), w)
				}
				if refresh != nil {
					refresh()
				}
			})
		}()
	}
}

func confirmRemoveTunnel(cfg Config, tunnelID string, w fyne.Window, refresh func()) {
	confirm := dialog.NewConfirm(
		"Remove Tunnel",
		fmt.Sprintf("Remove tunnel %q?", tunnelID),
		func(ok bool) {
			if !ok {
				return
			}
			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				dialog.ShowError(fmt.Errorf("load tunnel config: %w", err), w)
				return
			}

			tunnel, ok := tc.GetTunnel(tunnelID)
			if !ok {
				dialog.ShowError(fmt.Errorf("tunnel %q not found", tunnelID), w)
				return
			}

			if IsTunnelRunning(cfg, tunnel) {
				if err := StopTunnel(cfg, tunnel); err != nil {
					dialog.ShowError(fmt.Errorf("stop tunnel: %w", err), w)
					return
				}
			}

			if !tc.RemoveTunnel(tunnelID) {
				dialog.ShowError(fmt.Errorf("tunnel %q not found", tunnelID), w)
				return
			}

			if err := SaveTunnelConfig(tunnelConfigPath(cfg), tc); err != nil {
				dialog.ShowError(fmt.Errorf("save tunnel config: %w", err), w)
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

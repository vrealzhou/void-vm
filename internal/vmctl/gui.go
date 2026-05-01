package vmctl

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type guiModel struct {
	mu              sync.Mutex
	cfg             Config
	lastSample      *GuestMetricsSample
	busy            bool
	refreshing      bool
	running         bool
	bootstrapDone   bool
	lastAction      string
	lastStatusError string
}

func LaunchControlGUI() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	model := &guiModel{
		cfg:        cfg,
		lastAction: "Click Bootstrap first. That popup is where preferences are chosen.",
	}
	cfgPath := DotEnvPath(cfg.RepoRoot)

	a := app.NewWithID("github.com/vrealzhou/void-vm")
	w := a.NewWindow("void-vm control panel")
	w.Resize(fyne.NewSize(1080, 760))
	w.SetMaster()

	guideLabel := widget.NewRichTextFromMarkdown(fmt.Sprintf(
		"`Bootstrap` is the first step.\n\n1. Click `Bootstrap`\n2. Choose shell, editor, and window manager in the popup\n3. Run bootstrap once\n4. After that, use `Start` and `Stop`\n\nThe GUI writes bootstrap preferences to `%s`. After bootstrap, make further environment changes inside the VM directly.",
		cfgPath,
	))

	overviewLabel := widget.NewLabel("")
	overviewLabel.Wrapping = fyne.TextWrapWord
	overviewLabel.Selectable = true

	resourceLabel := widget.NewLabel("")
	resourceLabel.Wrapping = fyne.TextWrapWord
	resourceLabel.Selectable = true

	actionLabel := widget.NewLabel(model.lastAction)
	actionLabel.Wrapping = fyne.TextWrapWord

	refreshLabel := widget.NewLabel("Waiting for first refresh...")
	refreshLabel.Wrapping = fyne.TextWrapWord

	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	var (
		bootstrapButton *widget.Button
		startButton     *widget.Button
		stopButton      *widget.Button
		destroyButton   *widget.Button
	)

	setAction := func(message string) {
		model.mu.Lock()
		model.lastAction = message
		model.mu.Unlock()
		actionLabel.SetText(message)
	}

	applyButtonState := func() {
		model.mu.Lock()
		busy := model.busy
		running := model.running
		bootstrapDone := model.bootstrapDone
		model.mu.Unlock()

		if busy {
			bootstrapButton.Disable()
			startButton.Disable()
			stopButton.Disable()
			destroyButton.Disable()
			progress.Show()
			return
		}

		progress.Hide()
		bootstrapButton.Enable()
		destroyButton.Enable()
		if !bootstrapDone {
			startButton.Disable()
			stopButton.Disable()
			return
		}
		if running {
			startButton.Disable()
			stopButton.Enable()
			return
		}
		startButton.Enable()
		stopButton.Disable()
	}

	refreshView := func(status VMStatus, metrics *GuestMetrics) {
		overviewLabel.SetText(formatOverview(status))
		resourceLabel.SetText(formatResourceUsage(status, metrics))

		model.mu.Lock()
		model.running = status.Running
		model.bootstrapDone = status.BootstrapDone
		if model.lastStatusError != "" {
			refreshLabel.SetText("Last refresh: " + model.lastStatusError)
		} else {
			refreshLabel.SetText("Last refresh: " + time.Now().Format(time.RFC3339))
		}
		model.mu.Unlock()

		applyButtonState()
	}

	reloadConfig := func() {
		cfg, err := LoadConfig()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		model.mu.Lock()
		model.cfg = cfg
		model.mu.Unlock()
	}

	refreshStatus := func() {
		model.mu.Lock()
		if model.busy || model.refreshing {
			model.mu.Unlock()
			return
		}
		model.refreshing = true
		cfg := model.cfg
		previous := model.lastSample
		model.mu.Unlock()

		go func() {
			status, err := InspectVM(cfg)
			var metrics *GuestMetrics
			statusErr := ""

			if err == nil && status.Running {
				sample, sampleErr := SampleGuestMetrics(cfg)
				if sampleErr == nil {
					calculated := CalculateGuestMetrics(sample, previous)
					metrics = &calculated
					model.mu.Lock()
					model.lastSample = &sample
					model.mu.Unlock()
				} else {
					statusErr = "guest metrics unavailable: " + sampleErr.Error()
				}
			}
			if err != nil {
				statusErr = err.Error()
			}
			if !status.Running {
				model.mu.Lock()
				model.lastSample = nil
				model.mu.Unlock()
			}

			fyne.Do(func() {
				model.mu.Lock()
				model.refreshing = false
				model.lastStatusError = statusErr
				model.mu.Unlock()
				refreshView(status, metrics)
			})
		}()
	}

	runTask := func(label string, fn func(Config) error) {
		model.mu.Lock()
		if model.busy {
			model.mu.Unlock()
			return
		}
		model.busy = true
		cfg := model.cfg
		model.mu.Unlock()

		fyne.Do(func() {
			setAction(label + "...")
			applyButtonState()
		})

		go func() {
			err := fn(cfg)
			fyne.Do(func() {
				model.mu.Lock()
				model.busy = false
				model.mu.Unlock()

				if err != nil {
					setAction(label + " failed: " + err.Error())
					dialog.ShowError(err, w)
				} else {
					setAction(label + " complete.")
				}

				reloadConfig()
				refreshStatus()
			})
		}()
	}

	runBootstrapPopup := func() {
		model.mu.Lock()
		cfg := model.cfg
		model.mu.Unlock()

		shellSelect := widget.NewSelect([]string{"fish", "zsh"}, nil)
		shellSelect.SetSelected(cfg.DefaultShell)
		editorSelect := widget.NewSelect([]string{"neovim", "helix"}, nil)
		editorSelect.SetSelected(cfg.DefaultEditor)
		windowManagerSelect := widget.NewSelect([]string{"sway", "xfce"}, nil)
		windowManagerSelect.SetSelected(cfg.WindowManager)

		items := []*widget.FormItem{
			widget.NewFormItem("Default shell", shellSelect),
			widget.NewFormItem("Default editor", editorSelect),
			widget.NewFormItem("Window manager", windowManagerSelect),
		}

		dialog.ShowForm("Bootstrap Preferences", "Run Bootstrap", "Cancel", items, func(ok bool) {
			if !ok {
				return
			}

			updates := map[string]string{
				"VM_DEFAULT_SHELL":  shellSelect.Selected,
				"VM_DEFAULT_EDITOR": editorSelect.Selected,
				"VM_WINDOW_MANAGER": windowManagerSelect.Selected,
			}
			if err := UpdateDotEnvFile(cfgPath, updates); err != nil {
				dialog.ShowError(err, w)
				setAction("Saving bootstrap preferences failed: " + err.Error())
				return
			}

			reloadConfig()
			runTask("Running bootstrap", BootstrapSetup)
		}, w)
	}

	bootstrapButton = widget.NewButtonWithIcon("Bootstrap", theme.SettingsIcon(), runBootstrapPopup)
	bootstrapButton.Importance = widget.HighImportance

	startButton = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		runTask("Starting VM", Start)
	})

	stopButton = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		runTask("Stopping VM", Stop)
	})

	destroyButton = widget.NewButtonWithIcon("Destroy", theme.DeleteIcon(), func() {
		model.mu.Lock()
		cfg := model.cfg
		model.mu.Unlock()
		confirm := dialog.NewConfirm(
			"Destroy VM",
			fmt.Sprintf("Stop the VM and delete generated files under:\n\n%s\n\nThis keeps your base image under %s.", cfg.StateDir, cfg.ImageDir),
			func(ok bool) {
				if ok {
					runTask("Destroying VM", Destroy)
				}
			},
			w,
		)
		confirm.SetConfirmText("Destroy")
		confirm.SetConfirmImportance(widget.DangerImportance)
		confirm.Show()
	})

	buttonPane := container.NewVBox(
		widget.NewLabelWithStyle("Actions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, bootstrapButton, startButton),
		container.NewGridWithColumns(2, stopButton, destroyButton),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Activity", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		actionLabel,
		refreshLabel,
		progress,
	)

	statusPane := container.NewVBox(
		widget.NewLabelWithStyle("VM Status", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		overviewLabel,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Guest Resources", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		resourceLabel,
	)

	syncPanel := makeSyncPanel(cfg, w, func() {
		refreshStatus()
	})

	leftPane := container.NewVBox(
		guideLabel,
		widget.NewSeparator(),
		buttonPane,
		widget.NewSeparator(),
		syncPanel,
	)

	split := container.NewHSplit(leftPane, statusPane)
	split.Offset = 0.5
	w.SetContent(container.NewBorder(nil, nil, nil, nil, split))

	done := make(chan struct{})
	w.SetOnClosed(func() {
		close(done)
	})

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				refreshStatus()
			}
		}
	}()

	applyButtonState()
	refreshStatus()
	w.ShowAndRun()
	return nil
}

func formatOverview(status VMStatus) string {
	lines := []string{
		fmt.Sprintf("Name: %s", status.Name),
		fmt.Sprintf("State: %s", status.State),
		fmt.Sprintf("Running: %t", status.Running),
		fmt.Sprintf("Bootstrap complete: %t", status.BootstrapDone),
		fmt.Sprintf("Disk: %s", status.DiskPath),
		fmt.Sprintf("IP: %s", status.StaticIP),
		fmt.Sprintf("SSH: %s", status.SSHTarget),
	}
	if status.Running {
		lines = append(lines, fmt.Sprintf("PID: %d", status.PID))
	}
	return joinLines(lines...)
}

func formatResourceUsage(status VMStatus, metrics *GuestMetrics) string {
	if !status.Running {
		return "The VM is stopped."
	}
	if metrics == nil || !metrics.Available {
		return "Guest metrics are not available yet. SSH may still be starting."
	}

	cpuText := "CPU usage: waiting for a second sample"
	if metrics.HasCPUPercent {
		cpuText = fmt.Sprintf("CPU usage: %.1f%%", metrics.CPUPercent)
	}

	return joinLines(
		cpuText,
		fmt.Sprintf("Memory: %d MiB / %d MiB (%.1f%%)", metrics.MemUsedMiB, metrics.MemTotalMiB, metrics.MemUsedPercent),
		"Sampling source: /proc/stat and /proc/meminfo over SSH.",
	)
}

func joinLines(lines ...string) string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

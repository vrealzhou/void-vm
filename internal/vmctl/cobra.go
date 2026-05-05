package vmctl

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRootCommand() (*cobra.Command, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	rootCmd := &cobra.Command{
		Use:           "agent-vm",
		Short:         "Agent VM — a reproducible Void Linux dev VM on Apple Silicon",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetString("port")
			return LaunchWebServer(port)
		},
	}
	rootCmd.Flags().StringP("port", "p", "", "web UI port (default: 8080)")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			fmt.Fprint(cmd.OutOrStdout(), Usage(cfg))
		} else {
			defaultHelp(cmd, args)
		}
	})

	rootCmd.AddCommand(
		newStartCommand(cfg),
		newStopCommand(cfg),
		newDestroyCommand(cfg),
		newStatusCommand(cfg),
		newGUICommand(cfg),
		newBootstrapCommand(cfg),
		newClipInCommand(cfg),
		newClipOutCommand(cfg),
		newSSHCommand(cfg),
		newIPCommand(cfg),
		newSyncCommand(cfg),
		newTunnelCommand(cfg),
	)

	return rootCmd, nil
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

func newStartCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Create missing assets and start the VM",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Start(cfg)
		},
	}
}

func newStopCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the VM via vfkit REST API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Stop(cfg)
		},
	}
}

func newDestroyCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Stop the VM and remove generated VM state and disk files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Destroy(cfg)
		},
	}
}

func newStatusCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show VM state and effective network target",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Status(cfg)
		},
	}
}

func newBootstrapCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Configure fish + Starship + Rust + Homebrew + desktop tools inside the guest over SSH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return BootstrapSetup(cfg)
		},
	}
}

func newClipInCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "clip-in",
		Short: "Copy the macOS clipboard into the guest Wayland clipboard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ClipboardIn(cfg)
		},
	}
}

func newClipOutCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "clip-out",
		Short: "Copy the guest Wayland clipboard into the macOS clipboard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ClipboardOut(cfg)
		},
	}
}

func newSSHCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:                "ssh [ssh args...]",
		Short:              "SSH into the guest using the configured static IP",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return SSH(cfg, args)
		},
	}
}

func newIPCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "ip",
		Short: "Print the configured guest IP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), cfg.StaticIP)
			return nil
		},
	}
}

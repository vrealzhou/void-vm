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

func newStartCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Create missing assets and start the VM",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return Start(cfg)
		}),
	}
}

func newStopCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the VM via vfkit REST API",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return Stop(cfg)
		}),
	}
}

func newDestroyCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Stop the VM and remove generated VM state and disk files",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return Destroy(cfg)
		}),
	}
}

func newStatusCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show VM state and effective network target",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return Status(cfg)
		}),
	}
}

func newGUICommand(cfg Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Open the Web VM control panel",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return LaunchWebServer("")
		}),
	}
	cmd.Flags().String("port", "", "Server port (default: 8080 or VM_MANAGER_PORT env)")
	return cmd
}

func newBootstrapCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Configure fish + Starship + Rust + Homebrew + desktop tools inside the guest over SSH",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return BootstrapSetup(cfg)
		}),
	}
}

func newClipInCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "clip-in",
		Short: "Copy the macOS clipboard into the guest Wayland clipboard",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return ClipboardIn(cfg)
		}),
	}
}

func newClipOutCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "clip-out",
		Short: "Copy the guest Wayland clipboard into the macOS clipboard",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			return ClipboardOut(cfg)
		}),
	}
}

func newSSHCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:                "ssh [ssh args...]",
		Short:              "SSH into the guest using the configured static IP (vm@" + cfg.StaticIP + ")",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] == "help" {
				return cmd.Help()
			}
			for _, a := range args {
				if a == "--help" || a == "-h" {
					return cmd.Help()
				}
			}
			return SSH(cfg, args)
		},
	}
}

func newIPCommand(cfg Config) *cobra.Command {
	var setIP string
	cmd := &cobra.Command{
		Use:   "ip",
		Short: "Print or set the guest IP address",
		Args: leafArgs,
		RunE: leafRunE(func(cmd *cobra.Command, args []string) error {
			if setIP != "" {
				cfg.StaticIP = setIP
				if err := SaveConfig(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "IP updated to %s\n", setIP)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), cfg.StaticIP)
			return nil
		}),
	}
	cmd.Flags().StringVar(&setIP, "set", "", "set guest IP address in vmctl.yaml")
	return cmd
}

func leafRunE(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && args[0] == "help" {
			return cmd.Help()
		}
		return fn(cmd, args)
	}
}

func leafArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 1 && args[0] == "help" {
		return nil
	}
	return cobra.NoArgs(cmd, args)
}

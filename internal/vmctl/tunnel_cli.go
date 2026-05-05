package vmctl

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

func tunnelConfigPath(cfg Config) string {
	return filepath.Join(cfg.ConfigDir, "vmctl.yaml")
}

func newTunnelCommand(cfg Config) *cobra.Command {
	tunnelCmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Manage SSH tunnels",
	}

	tunnelCmd.AddCommand(
		newTunnelListCommand(cfg),
		newTunnelAddCommand(cfg),
		newTunnelStartCommand(cfg),
		newTunnelStopCommand(cfg),
		newTunnelRemoveCommand(cfg),
		newTunnelStartAllCommand(cfg),
		newTunnelStopAllCommand(cfg),
	)

	return tunnelCmd
}

func newTunnelListCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured tunnels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			if len(tc.Tunnels) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tunnels configured.")
				return nil
			}

			for _, t := range tc.Tunnels {
				status := "stopped"
				if IsTunnelRunning(cfg, t) {
					status = "running"
				}
				remoteHost := t.RemoteHost
				if remoteHost == "" {
					remoteHost = "localhost"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %s  local:%d -> %s:%d  auto-start:%t\n",
					t.ID, t.Name, status, t.Type, t.LocalPort, remoteHost, t.RemotePort, t.AutoStart)
			}
			return nil
		},
	}
}

func newTunnelAddCommand(cfg Config) *cobra.Command {
	var (
		name       string
		tunnelType string
		localPort  int
		remotePort int
		remoteHost string
		autoStart  bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new tunnel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if tunnelType == "" {
				return fmt.Errorf("--type is required")
			}
			if tunnelType != string(TunnelTypeLocal) && tunnelType != string(TunnelTypeRemote) {
				return fmt.Errorf("--type must be 'local' or 'remote'")
			}
			if localPort == 0 {
				return fmt.Errorf("--local-port is required")
			}
			if remotePort == 0 {
				return fmt.Errorf("--remote-port is required")
			}

			if remoteHost == "" {
				remoteHost = "localhost"
			}

			tunnel := Tunnel{
				ID:         name,
				Name:       name,
				Type:       TunnelType(tunnelType),
				LocalPort:  localPort,
				RemoteHost: remoteHost,
				RemotePort: remotePort,
				Enabled:    true,
				AutoStart:  autoStart,
				CreatedAt:  time.Now().UTC(),
			}

			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			if err := tc.AddTunnel(tunnel); err != nil {
				return err
			}

			if err := SaveTunnelConfig(tunnelConfigPath(cfg), tc); err != nil {
				return fmt.Errorf("save tunnel config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added tunnel %q\n", tunnel.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Tunnel name (required)")
	cmd.Flags().StringVar(&tunnelType, "type", "", "Tunnel type: local or remote (required)")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "Local port number (required)")
	cmd.Flags().IntVar(&remotePort, "remote-port", 0, "Remote port number (required)")
	cmd.Flags().StringVar(&remoteHost, "remote-host", "", "Remote host (default: localhost)")
	cmd.Flags().BoolVar(&autoStart, "auto-start", false, "Auto-start when VM runs")

	return cmd
}

func newTunnelStartCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "start <tunnel-id>",
		Short: "Start a tunnel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tunnelID := args[0]

			status, err := InspectVM(cfg)
			if err != nil {
				return fmt.Errorf("inspect VM: %w", err)
			}
			if !status.Running {
				return fmt.Errorf("VM is not running")
			}

			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			tunnel, ok := tc.GetTunnel(tunnelID)
			if !ok {
				return fmt.Errorf("tunnel %q not found", tunnelID)
			}

			if err := StartTunnel(cfg, tunnel); err != nil {
				return fmt.Errorf("start tunnel: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Started tunnel %q\n", tunnelID)
			return nil
		},
	}
}

func newTunnelStopCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <tunnel-id>",
		Short: "Stop a tunnel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tunnelID := args[0]

			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			tunnel, ok := tc.GetTunnel(tunnelID)
			if !ok {
				return fmt.Errorf("tunnel %q not found", tunnelID)
			}

			if err := StopTunnel(cfg, tunnel); err != nil {
				return fmt.Errorf("stop tunnel: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Stopped tunnel %q\n", tunnelID)
			return nil
		},
	}
}

func newTunnelRemoveCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <tunnel-id>",
		Short: "Remove a tunnel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tunnelID := args[0]

			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			tunnel, ok := tc.GetTunnel(tunnelID)
			if !ok {
				return fmt.Errorf("tunnel %q not found", tunnelID)
			}

			if IsTunnelRunning(cfg, tunnel) {
				if err := StopTunnel(cfg, tunnel); err != nil {
					return fmt.Errorf("stop tunnel: %w", err)
				}
			}

			if !tc.RemoveTunnel(tunnelID) {
				return fmt.Errorf("tunnel %q not found", tunnelID)
			}

			if err := SaveTunnelConfig(tunnelConfigPath(cfg), tc); err != nil {
				return fmt.Errorf("save tunnel config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed tunnel %q\n", tunnelID)
			return nil
		},
	}
}

func newTunnelStartAllCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "start-all",
		Short: "Start all enabled tunnels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := InspectVM(cfg)
			if err != nil {
				return fmt.Errorf("inspect VM: %w", err)
			}
			if !status.Running {
				return fmt.Errorf("VM is not running")
			}

			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			enabled := tc.GetEnabledTunnels()
			if len(enabled) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No enabled tunnels to start.")
				return nil
			}

			var started, skipped int
			for _, tunnel := range enabled {
				if IsTunnelRunning(cfg, tunnel) {
					skipped++
					continue
				}
				if err := StartTunnel(cfg, tunnel); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Failed to start tunnel %q: %v\n", tunnel.ID, err)
					continue
				}
				started++
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Started %d tunnel(s), skipped %d already running\n", started, skipped)
			return nil
		},
	}
}

func newTunnelStopAllCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "stop-all",
		Short: "Stop all tunnels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tc, err := LoadTunnelConfig(tunnelConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load tunnel config: %w", err)
			}

			if len(tc.Tunnels) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tunnels configured.")
				return nil
			}

			var stopped, skipped int
			for _, tunnel := range tc.Tunnels {
				if !IsTunnelRunning(cfg, tunnel) {
					skipped++
					continue
				}
				if err := StopTunnel(cfg, tunnel); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Failed to stop tunnel %q: %v\n", tunnel.ID, err)
					continue
				}
				stopped++
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Stopped %d tunnel(s), skipped %d not running\n", stopped, skipped)
			return nil
		},
	}
}

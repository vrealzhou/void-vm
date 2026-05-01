package vmctl

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func syncConfigPath(cfg Config) string {
	return filepath.Join(cfg.RepoRoot, ".vmctl.sync")
}

func syncBackupsDir(cfg Config) string {
	return filepath.Join(cfg.StateDir, "sync-backups")
}

func newSyncCommand(cfg Config) *cobra.Command {
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Manage file sync pairs between host and VM",
	}

	syncCmd.AddCommand(
		newSyncListCommand(cfg),
		newSyncAddGitCommand(cfg),
		newSyncAddCopyCommand(cfg),
		newSyncRunCommand(cfg),
		newSyncRemoveCommand(cfg),
		newSyncHistoryCommand(cfg),
		newSyncRestoreCommand(cfg),
	)

	return syncCmd
}

func newSyncListCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured sync pairs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			if len(sc.Pairs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No sync pairs configured.")
				return nil
			}

			for _, p := range sc.Pairs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %s\n", p.ID, p.Mode, p.HostPath, p.VMPath)
			}
			return nil
		},
	}
}

func newSyncAddGitCommand(cfg Config) *cobra.Command {
	var hostDir, vmDir, bareRepo string

	cmd := &cobra.Command{
		Use:   "add-git",
		Short: "Add a git sync pair",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hostDir == "" || vmDir == "" {
				return fmt.Errorf("--host-dir and --vm-dir are required")
			}

			if !IsGitRepo(hostDir) {
				return fmt.Errorf("host-dir %q is not a git repository", hostDir)
			}

			pair := SyncPair{
				ID:       filepath.Base(hostDir),
				Mode:     SyncModeGit,
				HostPath: hostDir,
				VMPath:   vmDir,
				CreatedAt: time.Now().UTC(),
			}

			if bareRepo != "" {
				pair.BareRepoPath = bareRepo
			} else {
				pair.BareRepoPath = defaultBareRepoPath(pair)
			}

			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			if err := sc.AddPair(pair); err != nil {
				return err
			}

			if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
				return fmt.Errorf("save sync config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added git sync pair %q\n", pair.ID)

			if err := GitSetupPair(cfg, pair); err != nil {
				return fmt.Errorf("git setup: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&hostDir, "host-dir", "", "Host directory (must be a git repo)")
	cmd.Flags().StringVar(&vmDir, "vm-dir", "", "VM target directory")
	cmd.Flags().StringVar(&bareRepo, "bare-repo", "", "Bare repo path on VM (default: auto-generated)")

	return cmd
}

func newSyncAddCopyCommand(cfg Config) *cobra.Command {
	var hostDir, vmDir, direction, exclude string
	var retention int

	cmd := &cobra.Command{
		Use:   "add-copy",
		Short: "Add a copy sync pair",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hostDir == "" || vmDir == "" {
				return fmt.Errorf("--host-dir and --vm-dir are required")
			}

			pair := SyncPair{
				ID:        filepath.Base(hostDir),
				Mode:      SyncModeCopy,
				HostPath:  hostDir,
				VMPath:    vmDir,
				Direction: SyncDirection(direction),
				CreatedAt: time.Now().UTC(),
			}

			if exclude != "" {
				pair.Exclude = strings.Split(exclude, ",")
			}

			if retention != 0 {
				pair.BackupRetentionDays = retention
			}

			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			if err := sc.AddPair(pair); err != nil {
				return err
			}

			if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
				return fmt.Errorf("save sync config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added copy sync pair %q\n", pair.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&hostDir, "host-dir", "", "Host directory")
	cmd.Flags().StringVar(&vmDir, "vm-dir", "", "VM target directory")
	cmd.Flags().StringVar(&direction, "direction", string(SyncDirectionHostToVM), "Sync direction: host-to-vm, vm-to-host, bidirectional")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Comma-separated exclude patterns")
	cmd.Flags().IntVar(&retention, "retention", 0, "Backup retention days (default: 7)")

	return cmd
}

func newSyncRunCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "run <pair-id>",
		Short: "Run sync for a specific pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairID := args[0]

			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			pair, ok := sc.GetPair(pairID)
			if !ok {
				return fmt.Errorf("sync pair %q not found", pairID)
			}

			switch pair.Mode {
			case SyncModeGit:
				fmt.Fprintln(cmd.OutOrStdout(), "Git sync pairs are managed via git commands. Use 'git push vm' and 'git pull vm' in your host repo.")
			case SyncModeCopy:
				switch pair.Direction {
				case SyncDirectionHostToVM:
					if err := CopySyncHostToVM(cfg, pair, syncBackupsDir(cfg)); err != nil {
						return fmt.Errorf("sync host-to-vm: %w", err)
					}
				case SyncDirectionVMToHost:
					if err := CopySyncVMToHost(cfg, pair, syncBackupsDir(cfg)); err != nil {
						return fmt.Errorf("sync vm-to-host: %w", err)
					}
				case SyncDirectionBidirectional:
					if err := CopySyncBidirectional(cfg, pair, syncBackupsDir(cfg)); err != nil {
						return fmt.Errorf("sync bidirectional: %w", err)
					}
				default:
					return fmt.Errorf("unknown direction: %s", pair.Direction)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Sync completed for %q\n", pairID)
			default:
				return fmt.Errorf("unknown sync mode: %s", pair.Mode)
			}

			return nil
		},
	}
}

func newSyncRemoveCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <pair-id>",
		Short: "Remove a sync pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairID := args[0]

			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			if !sc.RemovePair(pairID) {
				return fmt.Errorf("sync pair %q not found", pairID)
			}

			if err := SaveSyncConfig(syncConfigPath(cfg), sc); err != nil {
				return fmt.Errorf("save sync config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed sync pair %q\n", pairID)
			return nil
		},
	}
}

func newSyncHistoryCommand(cfg Config) *cobra.Command {
	return &cobra.Command{
		Use:   "history <pair-id>",
		Short: "Show backup history for a sync pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairID := args[0]

			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			if _, ok := sc.GetPair(pairID); !ok {
				return fmt.Errorf("sync pair %q not found", pairID)
			}

			backups, err := listBackups(syncBackupsDir(cfg), pairID)
			if err != nil {
				return fmt.Errorf("list backups: %w", err)
			}

			if len(backups) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No backups found.")
				return nil
			}

			for _, ts := range backups {
				fmt.Fprintln(cmd.OutOrStdout(), ts.Format(time.RFC3339))
			}
			return nil
		},
	}
}

func newSyncRestoreCommand(cfg Config) *cobra.Command {
	var timestamp string

	cmd := &cobra.Command{
		Use:   "restore <pair-id>",
		Short: "Restore from backup for a sync pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairID := args[0]

			if timestamp == "" {
				return fmt.Errorf("--timestamp is required")
			}

			sc, err := LoadSyncConfig(syncConfigPath(cfg))
			if err != nil {
				return fmt.Errorf("load sync config: %w", err)
			}

			pair, ok := sc.GetPair(pairID)
			if !ok {
				return fmt.Errorf("sync pair %q not found", pairID)
			}

			ts, err := time.Parse(time.RFC3339, timestamp)
			if err != nil {
				return fmt.Errorf("invalid timestamp: %w", err)
			}

			if err := restoreBackup(syncBackupsDir(cfg), pairID, ts, pair.HostPath); err != nil {
				return fmt.Errorf("restore backup: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Restored %q from backup %s\n", pairID, timestamp)
			return nil
		},
	}

	cmd.Flags().StringVar(&timestamp, "timestamp", "", "Backup timestamp (RFC3339 format)")

	return cmd
}

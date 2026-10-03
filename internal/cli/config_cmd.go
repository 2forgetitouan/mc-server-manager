package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"mc-server-manager/internal/config"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage/check configuration",
		RunE:  runConfig,
	}
	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigInitCmd(),
		newConfigPathCmd(),
	)
	return cmd
}

func runConfig(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE:  runConfigShow,
	}
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	path := config.FindConfigPath()
	if path == "" {
		fmt.Println(ui.Warning("No configuration file found."))
		fmt.Println(ui.Dim("Run 'mc config init' to create one."))
		return nil
	}

	fmt.Println(ui.Bold("Configuration") + ui.Dim(fmt.Sprintf(" (%s)", path)))
	fmt.Println()

	fmt.Println(ui.FormatKV("Server Path", cfg.ServerPath))
	fmt.Println()

	fmt.Println(ui.Bold("Server"))
	fmt.Println(ui.FormatKV("  Minecraft", cfg.Server.MinecraftVersion))
	fmt.Println(ui.FormatKV("  Loader", cfg.Server.Loader))
	fmt.Println(ui.FormatKV("  Loader Version", cfg.Server.LoaderVersion))
	fmt.Println(ui.FormatKV("  Java", cfg.Server.Java))
	fmt.Println(ui.FormatKV("  Memory", fmt.Sprintf("%s - %s", cfg.Server.MinMemory, cfg.Server.MaxMemory)))
	fmt.Println(ui.FormatKV("  Port", fmt.Sprintf("%d", cfg.Server.Port)))
	fmt.Println(ui.FormatKV("  JAR", cfg.Server.Jar))
	fmt.Println()

	fmt.Println(ui.Bold("Systemd"))
	fmt.Println(ui.FormatKV("  Unit", cfg.Systemd.Unit))
	fmt.Println(ui.FormatKV("  Stop Timeout", fmt.Sprintf("%ds", cfg.Systemd.StopTimeout)))
	fmt.Println()

	fmt.Println(ui.Bold("Console"))
	fmt.Println(ui.FormatKV("  Method", cfg.Console.Method))
	fmt.Println(ui.FormatKV("  RCON Host", cfg.Console.RCONHost))
	fmt.Println(ui.FormatKV("  RCON Port", fmt.Sprintf("%d", cfg.Console.RCONPort)))
	fmt.Println()

	fmt.Println(ui.Bold("Modrinth"))
	fmt.Println(ui.FormatKV("  Enabled", fmt.Sprintf("%v", cfg.Modrinth.Enabled)))
	fmt.Println(ui.FormatKV("  Timeout", fmt.Sprintf("%ds", cfg.Modrinth.Timeout)))
	fmt.Println()

	fmt.Println(ui.Bold("Mods"))
	fmt.Println(ui.FormatKV("  Directory", cfg.Mods.Directory))
	fmt.Println(ui.FormatKV("  Backup Dir", cfg.Mods.BackupDir))
	fmt.Println(ui.FormatKV("  Max Backups", fmt.Sprintf("%d", cfg.Mods.MaxBackups)))
	if len(cfg.Mods.Pinned) > 0 {
		fmt.Println(ui.FormatKV("  Pinned", fmt.Sprintf("%d mod(s)", len(cfg.Mods.Pinned))))
	}

	return nil
}

func newConfigInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default configuration file",
		RunE:  runConfigInit,
	}
	cmd.Flags().StringP("path", "p", "", "config file path (default: ~/.config/mc/config.toml)")
	return cmd
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("path")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		path = filepath.Join(home, ".config", "mc", "config.toml")
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config file already exists at %s\nUse a different path with --path or edit the existing file", path)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	defaultCfg := config.Default()
	if err := defaultCfg.Save(path); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Println(ui.Success("Configuration created: ") + path)
	fmt.Println(ui.Dim("Edit it to match your server setup."))
	return nil
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show configuration file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.FindConfigPath()
			if path == "" {
				fmt.Println(ui.Warning("No configuration file found."))
				return nil
			}
			fmt.Println(path)
			return nil
		},
	}
}

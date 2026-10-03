package cli

import (
	"fmt"

	"mc-server-manager/internal/console"
	"mc-server-manager/internal/systemd"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newConsoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "console",
		Short: "Open interactive server console",
		Long:  "Open an interactive console to view logs and send commands to the server via RCON.",
		RunE:  runConsole,
	}
}

func runConsole(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mgr := systemd.NewManager(cfg.Systemd.Unit)

	if !mgr.IsActive(ctx) {
		return fmt.Errorf("server is not running\nStart it with 'mc start' first")
	}

	password, err := cfg.ReadRCONPassword()
	if err != nil {
		return fmt.Errorf("failed to read RCON password: %w\nCheck that enable-rcon=true in server.properties", err)
	}

	rconAddr := fmt.Sprintf("%s:%d", cfg.Console.RCONHost, cfg.Console.RCONPort)

	fmt.Println(ui.Info("Opening interactive console..."))
	fmt.Println(ui.Dim("Type Minecraft commands. Press Ctrl+C or Ctrl+D to exit."))
	fmt.Println()

	c := console.New(console.Options{
		RCONAddress:  rconAddr,
		RCONPassword: password,
		SystemdUnit:  cfg.Systemd.Unit,
		LogLines:     cfg.Display.LogLines,
	})

	return c.Run(ctx)
}

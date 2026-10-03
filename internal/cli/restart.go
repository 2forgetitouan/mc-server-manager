package cli

import (
	"fmt"

	"mc-server-manager/internal/systemd"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the server",
		RunE:  runRestart,
	}
}

func runRestart(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mgr := systemd.NewManager(cfg.Systemd.Unit)

	if !mgr.UnitExists(ctx) {
		return fmt.Errorf("systemd unit %q not found", cfg.Systemd.Unit)
	}

	fmt.Println(ui.Info("Restarting Minecraft server..."))
	fmt.Println()

	status, err := mgr.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if status.State == systemd.StateRunning || status.State == systemd.StateStarting {
		fmt.Println(ui.Bold("Step 1/2: Stopping server"))
		if err := gracefulStop(ctx, mgr); err != nil {
			return fmt.Errorf("failed during stop phase: %w", err)
		}
		fmt.Println()
	} else {
		fmt.Println(ui.Dim("Server is not running, skipping stop phase."))
		fmt.Println()
	}

	fmt.Println(ui.Bold("Step 2/2: Starting server"))
	return runStart(cmd, args)
}

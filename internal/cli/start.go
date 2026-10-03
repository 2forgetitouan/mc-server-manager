package cli

import (
	"context"
	"fmt"
	"time"

	"mc-server-manager/internal/systemd"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the Minecraft server",
		RunE:  runStart,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mgr := systemd.NewManager(cfg.Systemd.Unit)

	if !mgr.UnitExists(ctx) {
		return fmt.Errorf("systemd unit %q not found\nRun 'mc doctor' to diagnose or install the service first", cfg.Systemd.Unit)
	}

	status, err := mgr.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if status.State == systemd.StateRunning {
		fmt.Println(ui.Warning("Server is already running") + ui.Dim(fmt.Sprintf(" (PID %d)", status.PID)))
		return nil
	}

	if status.State == systemd.StateStarting {
		fmt.Println(ui.Warning("Server is already starting..."))
		return nil
	}

	fmt.Print(ui.Info("Starting Minecraft server... "))

	if err := mgr.Start(ctx); err != nil {
		fmt.Println(ui.Error("FAILED"))
		return fmt.Errorf("failed to start service: %w", err)
	}

	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			fmt.Println(ui.Warning("TIMEOUT"))
			fmt.Println(ui.Warning("Service started but may still be initializing. Check with 'mc status'."))
			return nil
		case <-ticker.C:
			s, err := mgr.Status(ctx)
			if err != nil {
				continue
			}
			switch s.State {
			case systemd.StateRunning:
				fmt.Println(ui.Success("OK"))
				fmt.Println(ui.Success("● Server is running") + ui.Dim(fmt.Sprintf(" (PID %d)", s.PID)))
				return nil
			case systemd.StateFailed:
				fmt.Println(ui.Error("FAILED"))
				return fmt.Errorf("service failed to start. Check logs with 'mc logs'")
			}
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

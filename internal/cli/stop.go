package cli

import (
	"context"
	"fmt"
	"time"

	"mc-server-manager/internal/rcon"
	"mc-server-manager/internal/systemd"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the server gracefully",
		RunE:  runStop,
	}
	cmd.Flags().Bool("force", false, "force kill the server (last resort)")
	return cmd
}

func runStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mgr := systemd.NewManager(cfg.Systemd.Unit)
	force, _ := cmd.Flags().GetBool("force")

	status, err := mgr.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if status.State == systemd.StateStopped {
		fmt.Println(ui.Dim("Server is not running."))
		return nil
	}

	if force {
		fmt.Print(ui.Warning("Force stopping server... "))
		if err := mgr.Stop(ctx); err != nil {
			fmt.Println(ui.Error("FAILED"))
			return fmt.Errorf("failed to stop service: %w", err)
		}
		fmt.Println(ui.Success("OK"))
		return nil
	}

	if err := gracefulStop(ctx, mgr); err != nil {
		return err
	}

	return nil
}

func gracefulStop(ctx context.Context, mgr *systemd.Manager) error {
	fmt.Print(ui.Info("Sending stop command via RCON... "))

	password, err := cfg.ReadRCONPassword()
	if err != nil {
		fmt.Println(ui.Warning("SKIP"))
		fmt.Println(ui.Dim("  Could not read RCON password, falling back to systemctl stop"))
		return systemdStop(ctx, mgr)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Console.RCONHost, cfg.Console.RCONPort)
	client, err := rcon.Dial(addr, password, 5*time.Second)
	if err != nil {
		fmt.Println(ui.Warning("SKIP"))
		fmt.Println(ui.Dim("  RCON connection failed, falling back to systemctl stop"))
		return systemdStop(ctx, mgr)
	}

	_, err = client.Execute("stop")
	client.Close()
	if err != nil {
		fmt.Println(ui.Warning("SKIP"))
		fmt.Println(ui.Dim("  RCON command failed, falling back to systemctl stop"))
		return systemdStop(ctx, mgr)
	}
	fmt.Println(ui.Success("OK"))

	return waitForStop(ctx, mgr)
}

func systemdStop(ctx context.Context, mgr *systemd.Manager) error {
	fmt.Print(ui.Info("Stopping via systemctl... "))
	if err := mgr.Stop(ctx); err != nil {
		fmt.Println(ui.Error("FAILED"))
		return fmt.Errorf("failed to stop service: %w", err)
	}
	fmt.Println(ui.Success("OK"))
	return waitForStop(ctx, mgr)
}

func waitForStop(ctx context.Context, mgr *systemd.Manager) error {
	fmt.Print(ui.Dim("Waiting for server to stop... "))

	timeout := time.Duration(cfg.Systemd.StopTimeout) * time.Second
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			fmt.Println(ui.Warning("TIMEOUT"))
			fmt.Println(ui.Warning(fmt.Sprintf("Server did not stop within %s.", timeout)))
			fmt.Println(ui.Dim("Use 'mc stop --force' to force kill."))
			return fmt.Errorf("stop timed out after %s", timeout)
		case <-ticker.C:
			s, err := mgr.Status(ctx)
			if err != nil {
				continue
			}
			if s.State == systemd.StateStopped || s.State == systemd.StateFailed {
				fmt.Println(ui.Success("OK"))
				fmt.Println(ui.Success("○ Server stopped."))
				return nil
			}
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
		return fmt.Errorf("systemd unit %q not found\nRun 'sudo mc service install' to install the service", cfg.Systemd.Unit)
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
		fmt.Println()
		showRecentLogs(ctx, mgr, 15)
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Quick check - if the service already failed, show logs immediately.
	time.Sleep(300 * time.Millisecond)
	s, err := mgr.Status(ctx)
	if err == nil && s.State == systemd.StateFailed {
		fmt.Println(ui.Error("FAILED"))
		fmt.Println()
		showRecentLogs(ctx, mgr, 20)
		return fmt.Errorf("service failed to start - see logs above")
	}

	fmt.Println(ui.Success("OK"))
	fmt.Println()

	return streamStartupLogs(ctx, mgr)
}

// streamStartupLogs follows journalctl output while waiting for the server to
// be fully started (or to fail). It prints each log line in real time so the
// user sees exactly what the JVM is doing during startup.
func streamStartupLogs(ctx context.Context, mgr *systemd.Manager) error {
	logCmd, err := mgr.Logs(ctx, 20, "", true)
	if err != nil {
		return fmt.Errorf("failed to start log stream: %w", err)
	}

	stdout, err := logCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe logs: %w", err)
	}
	logCmd.Stderr = os.Stderr

	if err := logCmd.Start(); err != nil {
		return fmt.Errorf("failed to start journalctl: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Channel for log lines from the scanner goroutine.
	lineCh := make(chan string, 64)
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
	}()

	// Poll service state every second while streaming logs.
	stateTicker := time.NewTicker(1 * time.Second)
	defer stateTicker.Stop()

	timeout := time.After(90 * time.Second)
	serverReady := false

	for {
		select {
		case line, ok := <-lineCh:
			if !ok {
				// journalctl ended - service probably died.
				goto done
			}
			fmt.Println(colorizeLine(line))

			// Detect common Minecraft "server ready" messages.
			if isServerReady(line) {
				serverReady = true
				goto done
			}

		case <-stateTicker.C:
			s, err := mgr.Status(ctx)
			if err != nil {
				continue
			}
			if s.State == systemd.StateFailed || s.State == systemd.StateStopped {
				// Drain remaining log lines before reporting failure.
				drainLines(lineCh, 50*time.Millisecond)
				fmt.Println()
				fmt.Println(ui.Error("Server failed to start."))
				logCmd.Process.Signal(syscall.SIGTERM)
				logCmd.Wait()
				return fmt.Errorf("service failed - check logs above")
			}

		case <-timeout:
			fmt.Println()
			fmt.Println(ui.Warning("Still starting... detaching from logs."))
			fmt.Println(ui.Dim("Follow progress with: mc logs"))
			logCmd.Process.Signal(syscall.SIGTERM)
			logCmd.Wait()
			return nil

		case <-sigCh:
			fmt.Println()
			fmt.Println(ui.Dim("Detached from logs. Server is still starting in the background."))
			logCmd.Process.Signal(syscall.SIGTERM)
			logCmd.Wait()
			return nil

		case <-ctx.Done():
			logCmd.Process.Signal(syscall.SIGTERM)
			logCmd.Wait()
			return context.Cause(ctx)
		}
	}

done:
	logCmd.Process.Signal(syscall.SIGTERM)
	logCmd.Wait()

	fmt.Println()
	if serverReady {
		s, _ := mgr.Status(ctx)
		pid := 0
		if s != nil {
			pid = s.PID
		}
		if pid > 0 {
			fmt.Println(ui.Success("Server is ready!") + ui.Dim(fmt.Sprintf(" (PID %d)", pid)))
		} else {
			fmt.Println(ui.Success("Server is ready!"))
		}
	} else {
		fmt.Println(ui.Dim("Log stream ended. Check status with: mc status"))
	}

	return nil
}

// isServerReady checks if a log line indicates the Minecraft server has
// finished loading and is ready to accept connections.
func isServerReady(line string) bool {
	lower := strings.ToLower(line)
	// Vanilla / Fabric / Paper / Spigot all emit one of these.
	readyPhrases := []string{
		"done (",           // "Done (12.345s)! For help, type "help""
		"server started",   // Some mods log this
		"listening on port", // Velocity proxy
	}
	for _, phrase := range readyPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// showRecentLogs prints the last N lines from the journal to help diagnose
// startup failures.
func showRecentLogs(ctx context.Context, mgr *systemd.Manager, n int) {
	lines, err := mgr.LogLines(ctx, n, "")
	if err != nil {
		fmt.Println(ui.Dim("  (could not read logs: " + err.Error() + ")"))
		return
	}
	if len(lines) == 0 {
		fmt.Println(ui.Dim("  (no log output found)"))
		return
	}
	fmt.Println(ui.Bold("Recent logs:"))
	for _, line := range lines {
		fmt.Println("  " + colorizeLine(line))
	}
}

// drainLines reads remaining lines from the channel for up to the given duration.
func drainLines(ch <-chan string, d time.Duration) {
	timer := time.After(d)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			fmt.Println(colorizeLine(line))
		case <-timer:
			return
		}
	}
}

package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mc-server-manager/internal/systemd"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View and follow server logs",
		RunE:  runLogs,
	}
	cmd.Flags().IntP("lines", "n", 0, "number of lines to show (default from config)")
	cmd.Flags().String("since", "", "show logs since (e.g., 1h, today, 2024-01-01)")
	cmd.Flags().Bool("no-follow", false, "don't follow new logs")
	return cmd
}

func runLogs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mgr := systemd.NewManager(cfg.Systemd.Unit)

	lines, _ := cmd.Flags().GetInt("lines")
	since, _ := cmd.Flags().GetString("since")
	noFollow, _ := cmd.Flags().GetBool("no-follow")

	if lines == 0 {
		lines = cfg.Display.LogLines
	}

	follow := !noFollow

	if !follow {
		logLines, err := mgr.LogLines(ctx, lines, since)
		if err != nil {
			return fmt.Errorf("failed to read logs: %w", err)
		}
		for _, line := range logLines {
			fmt.Println(colorizeLine(line))
		}
		return nil
	}

	logCmd, err := mgr.Logs(ctx, lines, since, true)
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

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			fmt.Println(colorizeLine(scanner.Text()))
		}
	}()

	select {
	case <-sigCh:
		logCmd.Process.Signal(syscall.SIGTERM)
	case <-done:
	}

	logCmd.Wait()
	return nil
}

func colorizeLine(line string) string {
	if !ui.ColorEnabled {
		return line
	}
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "severe"):
		return ui.Error(line)
	case strings.Contains(lower, "warn"):
		return ui.Warning(line)
	case strings.Contains(lower, "info"):
		return line
	default:
		return line
	}
}

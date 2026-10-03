package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"mc-server-manager/internal/rcon"
	"mc-server-manager/internal/stats"
	"mc-server-manager/internal/systemd"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show server status and resources",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	mgr := systemd.NewManager(cfg.Systemd.Unit)

	status, err := mgr.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	fmt.Println(ui.Bold("Minecraft Server Status"))
	fmt.Println(ui.Dim(strings.Repeat("─", 40)))
	fmt.Println()

	stateStr := string(status.State)
	icon := ui.StatusIcon(stateStr)
	fmt.Println(ui.FormatKV("State", icon+" "+stateStr))

	if status.PID > 0 {
		fmt.Println(ui.FormatKV("PID", fmt.Sprintf("%d", status.PID)))
	}

	if !status.ActiveSince.IsZero() && status.State == systemd.StateRunning {
		uptime := time.Since(status.ActiveSince)
		fmt.Println(ui.FormatKV("Uptime", stats.FormatDuration(uptime)))
	}

	fmt.Println()
	fmt.Println(ui.Bold("Server"))
	fmt.Println(ui.Dim(strings.Repeat("─", 40)))
	fmt.Println(ui.FormatKV("Version", cfg.Server.MinecraftVersion))
	fmt.Println(ui.FormatKV("Loader", cfg.Server.Loader))
	if cfg.Server.LoaderVersion != "" {
		fmt.Println(ui.FormatKV("Loader Version", cfg.Server.LoaderVersion))
	}
	fmt.Println(ui.FormatKV("Port", fmt.Sprintf("%d", cfg.Server.Port)))
	fmt.Println(ui.FormatKV("Path", cfg.ServerPath))

	modsDir := cfg.ModsPath()
	modCount := countMods(modsDir)
	if modCount >= 0 {
		fmt.Println(ui.FormatKV("Mods", fmt.Sprintf("%d", modCount)))
	}

	if status.PID > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("Resources"))
		fmt.Println(ui.Dim(strings.Repeat("─", 40)))

		procStats, err := stats.GetProcessStats(status.PID)
		if err == nil {
			fmt.Println(ui.FormatKV("Memory (RSS)", stats.FormatBytes(procStats.MemoryRSS)))
			fmt.Println(ui.FormatKV("Memory (VMS)", stats.FormatBytes(procStats.MemoryVMS)))
			fmt.Println(ui.FormatKV("Threads", fmt.Sprintf("%d", procStats.NumThreads)))
		}

		cpuPct, err := stats.MeasureCPU(status.PID)
		if err == nil {
			fmt.Println(ui.FormatKV("CPU", fmt.Sprintf("%.1f%%", cpuPct)))
		}

		sysStats, err := stats.GetSystemStats(cfg.ServerPath)
		if err == nil {
			memPct := float64(0)
			if sysStats.TotalMemory > 0 {
				if procStats != nil {
					memPct = float64(procStats.MemoryRSS) / float64(sysStats.TotalMemory) * 100
				}
			}
			fmt.Println(ui.FormatKV("RAM Usage", fmt.Sprintf("%.1f%% of %s", memPct, stats.FormatBytes(sysStats.TotalMemory))))
			fmt.Println(ui.FormatKV("Disk", fmt.Sprintf("%s free / %s",
				stats.FormatBytes(sysStats.DiskFree), stats.FormatBytes(sysStats.DiskTotal))))
		}

		fmt.Println(ui.FormatKV("Java Memory", fmt.Sprintf("%s - %s", cfg.Server.MinMemory, cfg.Server.MaxMemory)))
	}

	if status.State == systemd.StateRunning {
		players := getPlayerCount(ctx)
		if players != "" {
			fmt.Println()
			fmt.Println(ui.Bold("Players"))
			fmt.Println(ui.Dim(strings.Repeat("─", 40)))
			fmt.Println(ui.FormatKV("Online", players))
		}
	}

	if status.State == systemd.StateFailed {
		fmt.Println()
		fmt.Println(ui.Error("⚠ Service is in failed state"))
		if status.ExitCode != "" {
			fmt.Println(ui.FormatKV("Exit Code", status.ExitCode))
		}
		fmt.Println(ui.Dim("Run 'mc logs' to see what happened."))
	}

	return nil
}

func countMods(modsDir string) int {
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return -1
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jar") {
			count++
		}
	}
	return count
}

func getPlayerCount(_ any) string {
	password, err := cfg.ReadRCONPassword()
	if err != nil {
		return ""
	}
	addr := fmt.Sprintf("%s:%d", cfg.Console.RCONHost, cfg.Console.RCONPort)
	client, err := rcon.Dial(addr, password, 3*time.Second)
	if err != nil {
		return ""
	}
	defer client.Close()

	resp, err := client.Execute("list")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(resp)
}

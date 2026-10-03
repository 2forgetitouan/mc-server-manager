package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"mc-server-manager/internal/config"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the systemd service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newServiceInstallCmd(),
		newServiceGenerateEnvCmd(),
		newServiceUninstallCmd(),
	)
	return cmd
}

func newServiceInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install systemd service, environment file, and polkit rule",
		Long: `Install the systemd service unit, generate the JVM environment file
from config.toml, and install a polkit rule so the configured user can
manage the service without sudo/password.`,
		RunE: runServiceInstall,
	}
	cmd.Flags().String("service-src", "", "path to minecraft.service template (auto-detected)")
	return cmd
}

func runServiceInstall(cmd *cobra.Command, args []string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (try: sudo mc service install)")
	}

	serviceSrc, _ := cmd.Flags().GetString("service-src")
	if serviceSrc == "" {
		serviceSrc = findServiceFile()
		if serviceSrc == "" {
			return fmt.Errorf("cannot find minecraft.service template\n" +
				"Specify it with --service-src or run from the project directory")
		}
	}

	// 1. Generate environment file
	fmt.Print(ui.Info("Generating environment file... "))
	envPath := config.DefaultEnvFilePath()
	if err := cfg.GenerateEnvironmentFile(envPath); err != nil {
		fmt.Println(ui.Error("FAILED"))
		return fmt.Errorf("failed to generate environment file: %w", err)
	}
	fmt.Println(ui.Success("OK"))
	fmt.Println(ui.Dim("  " + envPath))

	// 2. Install systemd service
	fmt.Print(ui.Info("Installing systemd service... "))
	destService := filepath.Join("/etc/systemd/system", cfg.Systemd.Unit)
	data, err := os.ReadFile(serviceSrc)
	if err != nil {
		fmt.Println(ui.Error("FAILED"))
		return fmt.Errorf("reading service file: %w", err)
	}
	if err := os.WriteFile(destService, data, 0o644); err != nil {
		fmt.Println(ui.Error("FAILED"))
		return fmt.Errorf("writing service file: %w", err)
	}
	fmt.Println(ui.Success("OK"))
	fmt.Println(ui.Dim("  " + destService))

	// 3. Install polkit rule
	fmt.Print(ui.Info("Installing polkit rule... "))
	if err := installPolkitRule(); err != nil {
		fmt.Println(ui.Warning("SKIP"))
		fmt.Println(ui.Dim("  " + err.Error()))
	} else {
		fmt.Println(ui.Success("OK"))
	}

	// 4. Reload systemd
	fmt.Print(ui.Info("Reloading systemd... "))
	reload := exec.Command("systemctl", "daemon-reload")
	if out, err := reload.CombinedOutput(); err != nil {
		fmt.Println(ui.Error("FAILED"))
		return fmt.Errorf("daemon-reload failed: %w\n%s", err, string(out))
	}
	fmt.Println(ui.Success("OK"))

	// 5. Enable service
	fmt.Print(ui.Info("Enabling service... "))
	enable := exec.Command("systemctl", "enable", cfg.Systemd.Unit)
	if out, err := enable.CombinedOutput(); err != nil {
		fmt.Println(ui.Warning("SKIP"))
		fmt.Println(ui.Dim("  " + string(out)))
	} else {
		fmt.Println(ui.Success("OK"))
	}

	fmt.Println()
	fmt.Println(ui.Success("Service installed successfully!"))
	fmt.Println()
	fmt.Println(ui.Dim("The user can now run 'mc start/stop/restart' without sudo."))
	fmt.Println(ui.Dim("If you change memory/java/jar in config.toml, run:"))
	fmt.Println(ui.Dim("  sudo mc service generate-env && mc restart"))

	return nil
}

func newServiceGenerateEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate-env",
		Short: "Regenerate the JVM environment file from config.toml",
		Long: `Reads memory, java path, jar name, and JVM args from config.toml
and writes them to the systemd environment file. Run this after changing
these values in config.toml, then restart the server.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getuid() != 0 {
				return fmt.Errorf("this command must be run as root (try: sudo mc service generate-env)")
			}

			envPath := config.DefaultEnvFilePath()
			if err := cfg.GenerateEnvironmentFile(envPath); err != nil {
				return fmt.Errorf("failed to generate environment file: %w", err)
			}

			fmt.Println(ui.Success("Environment file generated: ") + envPath)
			fmt.Println()
			fmt.Println(ui.FormatKV("Java", cfg.Server.Java))
			fmt.Println(ui.FormatKV("JAR", cfg.Server.Jar))
			fmt.Println(ui.FormatKV("Memory", fmt.Sprintf("%s - %s", cfg.Server.MinMemory, cfg.Server.MaxMemory)))
			jvmArgs := cfg.JVMArgsList()
			fmt.Println(ui.FormatKV("JVM Args", fmt.Sprintf("%d flags", len(jvmArgs))))
			fmt.Println()
			fmt.Println(ui.Dim("Restart the server to apply: mc restart"))
			return nil
		},
	}
}

func newServiceUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove systemd service and polkit rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getuid() != 0 {
				return fmt.Errorf("this command must be run as root (try: sudo mc service uninstall)")
			}

			// Stop and disable
			exec.Command("systemctl", "stop", cfg.Systemd.Unit).Run()
			exec.Command("systemctl", "disable", cfg.Systemd.Unit).Run()

			servicePath := filepath.Join("/etc/systemd/system", cfg.Systemd.Unit)
			if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
				fmt.Println(ui.Warning("Could not remove service: " + err.Error()))
			} else {
				fmt.Println(ui.Success("Removed: ") + servicePath)
			}

			envPath := config.DefaultEnvFilePath()
			if err := os.Remove(envPath); err != nil && !os.IsNotExist(err) {
				fmt.Println(ui.Warning("Could not remove env file: " + err.Error()))
			} else {
				fmt.Println(ui.Success("Removed: ") + envPath)
			}

			removePolkitRule()

			exec.Command("systemctl", "daemon-reload").Run()
			fmt.Println(ui.Success("Systemd reloaded."))

			fmt.Println()
			fmt.Println(ui.Dim("Config and Minecraft data were NOT removed."))
			return nil
		},
	}
}

func findServiceFile() string {
	candidates := []string{
		"systemd/minecraft.service",
		"/usr/share/mc-server-manager/minecraft.service",
	}
	exe, err := os.Executable()
	if err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "..", "systemd", "minecraft.service"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func installPolkitRule() error {
	rulesDir := "/etc/polkit-1/rules.d"
	pklaDir := "/etc/polkit-1/localauthority/50-local.d"

	// Try modern rules.d format first
	if _, err := os.Stat(filepath.Dir(rulesDir)); err == nil {
		if err := os.MkdirAll(rulesDir, 0o755); err == nil {
			rule := `// Generated by mc-server-manager
polkit.addRule(function(action, subject) {
    if (action.id === "org.freedesktop.systemd1.manage-units" &&
        action.lookup("unit") === "minecraft.service" &&
        subject.user === "ubuntu") {
        return polkit.Result.YES;
    }
});
`
			dest := filepath.Join(rulesDir, "10-mc-minecraft.rules")
			if err := os.WriteFile(dest, []byte(rule), 0o644); err != nil {
				return fmt.Errorf("writing polkit rule: %w", err)
			}
			fmt.Println(ui.Dim("  " + dest))
			return nil
		}
	}

	// Fall back to legacy pkla format
	if runtime.GOOS == "linux" {
		if err := os.MkdirAll(pklaDir, 0o755); err == nil {
			pkla := `[Allow minecraft user to manage minecraft.service]
Identity=unix-user:ubuntu
Action=org.freedesktop.systemd1.manage-units
ResultAny=yes
ResultInactive=yes
ResultActive=yes
`
			dest := filepath.Join(pklaDir, "10-mc-minecraft.pkla")
			if err := os.WriteFile(dest, []byte(pkla), 0o644); err != nil {
				return fmt.Errorf("writing polkit pkla: %w", err)
			}
			fmt.Println(ui.Dim("  " + dest))
			return nil
		}
	}

	return fmt.Errorf("could not find polkit rules directory; configure polkit manually")
}

func removePolkitRule() {
	paths := []string{
		"/etc/polkit-1/rules.d/10-mc-minecraft.rules",
		"/etc/polkit-1/localauthority/50-local.d/10-mc-minecraft.pkla",
	}
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			fmt.Println(ui.Success("Removed: ") + p)
		}
	}
}

package cli

import (
	"fmt"
	"os"

	"mc-server-manager/internal/config"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

var (
	appVersion string
	cfg        *config.Config
	colorFlag  string
)

func SetVersion(v string) {
	appVersion = v
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "mc",
		Short: "Minecraft Server Manager",
		Long: `Minecraft Server Manager — a modern CLI tool to manage
a single Minecraft server with systemd on Linux.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ui.InitColors(colorFlag)

			if cmd.Name() == "version" || cmd.Name() == "help" {
				return nil
			}

			var err error
			cfg, err = config.Load()
			if err != nil {
				if cmd.Name() == "doctor" || cmd.Name() == "config" {
					cfg = config.Default()
					return nil
				}
				return fmt.Errorf("failed to load config: %w\nRun 'mc doctor' to diagnose or 'mc config init' to create one", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&colorFlag, "color", "auto", "color output: auto, always, never")

	root.SetHelpTemplate(helpTemplate())
	root.SetUsageTemplate(usageTemplate())

	root.AddCommand(
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newConsoleCmd(),
		newUpdateCmd(),
		newDoctorCmd(),
		newConfigCmd(),
		newServiceCmd(),
		newVersionCmd(),
	)

	return root
}

func Execute() error {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, ui.Error("Error: ")+err.Error())
		return err
	}
	return nil
}

func helpTemplate() string {
	return `{{with .Long}}{{. | trimTrailingWhitespaces}}{{end}}

{{.UsageString}}`
}

func usageTemplate() string {
	return `Usage:
  {{.CommandPath}} <command> [flags]

Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding}}  {{.Short}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

Use "{{.CommandPath}} <command> --help" for more information about a command.
`
}

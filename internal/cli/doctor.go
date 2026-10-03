package cli

import (
	"fmt"

	"mc-server-manager/internal/doctor"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose installation",
		Long:  "Run diagnostic checks on the Minecraft server installation.",
		RunE:  runDoctor,
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	d := doctor.New(cfg)

	fmt.Println(ui.Bold("Minecraft Server Diagnostics"))
	fmt.Println()

	results := d.RunAll(ctx)

	passCount := 0
	warnCount := 0
	failCount := 0

	for _, r := range results {
		var icon string
		switch r.Status {
		case doctor.Pass:
			icon = ui.Success("PASS")
			passCount++
		case doctor.Warn:
			icon = ui.Warning("WARN")
			warnCount++
		case doctor.Fail:
			icon = ui.Error("FAIL")
			failCount++
		}
		fmt.Printf("  [%s] %s\n", icon, r.Name)
		fmt.Printf("         %s\n", r.Message)
		if r.Fix != "" {
			fmt.Printf("         %s\n", ui.Dim("Fix: "+r.Fix))
		}
	}

	fmt.Println()
	fmt.Printf("Results: %s  %s  %s\n",
		ui.Success(fmt.Sprintf("%d passed", passCount)),
		ui.Warning(fmt.Sprintf("%d warnings", warnCount)),
		ui.Error(fmt.Sprintf("%d failed", failCount)),
	)

	if failCount > 0 {
		return fmt.Errorf("%d check(s) failed", failCount)
	}
	return nil
}

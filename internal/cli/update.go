package cli

import (
	"fmt"
	"strings"

	"mc-server-manager/internal/mods"
	"mc-server-manager/internal/ui"

	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check/apply mod updates",
		Long:  "Check for mod updates from Modrinth and optionally apply them.",
		RunE:  runUpdate,
	}
	cmd.Flags().Bool("check", false, "only check for updates, don't apply")
	cmd.Flags().Bool("dry-run", false, "alias for --check")
	cmd.Flags().BoolP("yes", "y", false, "skip confirmation prompt")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	checkOnly, _ := cmd.Flags().GetBool("check")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	autoYes, _ := cmd.Flags().GetBool("yes")

	if dryRun {
		checkOnly = true
	}

	if !cfg.Modrinth.Enabled {
		return fmt.Errorf("modrinth integration is disabled in configuration\n" +
			"enable it under [modrinth] in config.toml (not needed for vanilla servers)")
	}

	mgr := mods.NewManager(cfg)

	fmt.Println(ui.Info("Checking for mod updates..."))
	fmt.Println(ui.Dim(fmt.Sprintf("  Minecraft %s / %s", cfg.Server.MinecraftVersion, cfg.Server.Loader)))
	fmt.Println()

	updates, err := mgr.CheckUpdates(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if len(updates) == 0 {
		fmt.Println(ui.Success("All mods are up to date!"))
		return nil
	}

	fmt.Println(ui.Bold(fmt.Sprintf("Found %d update(s):", len(updates))))
	fmt.Println()

	headers := []string{"Mod", "Current", "Available", "File"}
	var rows [][]string
	for _, u := range updates {
		currentShort := u.CurrentFile
		if len(currentShort) > 30 {
			currentShort = currentShort[:27] + "..."
		}
		newFile := ""
		if len(u.LatestVersion.Files) > 0 {
			newFile = u.LatestFile.Filename
			if len(newFile) > 30 {
				newFile = newFile[:27] + "..."
			}
		}
		rows = append(rows, []string{
			u.ProjectTitle,
			currentShort,
			u.LatestVersion.VersionNumber,
			newFile,
		})
	}
	ui.PrintTable(headers, rows)
	fmt.Println()

	for _, u := range updates {
		if len(u.LatestVersion.Dependencies) > 0 {
			var deps []string
			for _, d := range u.LatestVersion.Dependencies {
				if d.Type == "required" && d.ProjectID != nil {
					deps = append(deps, *d.ProjectID)
				}
			}
			if len(deps) > 0 {
				fmt.Println(ui.Dim(fmt.Sprintf("  %s requires: %s", u.ProjectTitle, strings.Join(deps, ", "))))
			}
		}
	}

	if checkOnly {
		fmt.Println(ui.Dim("Run 'mc update' to apply these updates."))
		return nil
	}

	if !autoYes {
		fmt.Print(ui.Warning("Apply updates? [y/N] "))
		var answer string
		fmt.Scanln(&answer)
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println(ui.Dim("Aborted."))
			return nil
		}
	}

	fmt.Println()
	fmt.Println(ui.Info("Applying updates..."))
	fmt.Println()

	results, err := mgr.ApplyUpdates(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to apply updates: %w", err)
	}

	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
			fmt.Println(ui.Success("  ✓ ") + r.ModName + ui.Dim(fmt.Sprintf(" → %s", r.NewVersion)))
		} else {
			failCount++
			fmt.Println(ui.Error("  ✗ ") + r.ModName + ui.Dim(fmt.Sprintf(": %v", r.Error)))
		}
	}

	fmt.Println()
	if failCount == 0 {
		fmt.Println(ui.Success(fmt.Sprintf("All %d mod(s) updated successfully!", successCount)))
	} else {
		fmt.Println(ui.Warning(fmt.Sprintf("%d updated, %d failed.", successCount, failCount)))
	}

	if successCount > 0 {
		fmt.Println(ui.Dim("Restart the server to apply changes: mc restart"))
	}

	return nil
}

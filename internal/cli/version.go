package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show manager version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mc-server-manager %s\n", appVersion)
			fmt.Printf("  go:   %s\n", runtime.Version())
			fmt.Printf("  os:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

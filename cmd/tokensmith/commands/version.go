package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// These will be set by build flags
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// NewVersionCmd creates the version command.
func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tokensmith version %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", date)
		},
	}

	return cmd
}

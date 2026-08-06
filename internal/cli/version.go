package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zarxor/scripts/internal/version"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Johan Bostrom CLI %s\n", version.Version)
			return err
		},
	}
}

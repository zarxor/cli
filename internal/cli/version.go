package cli

import (
	"github.com/spf13/cobra"
	"github.com/zarxor/scripts/internal/render"
	"github.com/zarxor/scripts/internal/version"
)

func newVersionCommand(themeFor themeFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return render.NewRenderer(cmd.OutOrStdout(), themeFor(cmd)).Version("Johan Bostrom CLI", version.Version)
		},
	}
}

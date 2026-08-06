package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zarxor/scripts/internal/render"
	"github.com/zarxor/scripts/internal/selfupdate"
)

func newUpdateCommand(themeFor themeFactory) *cobra.Command {
	var dryRun bool

	command := &cobra.Command{
		Use:   "update",
		Short: "Update the Johan Bostrom CLI",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			writer := command.OutOrStdout()
			renderer := render.NewRenderer(writer, themeFor(command))
			if err := renderer.Progress("Checking for the latest Johan Bostrom CLI release…"); err != nil {
				return fmt.Errorf("render update progress: %w", err)
			}
			result, err := selfupdate.Run(command.Context(), selfupdate.Options{
				DryRun:   dryRun,
				Progress: renderer.Progress,
			})
			if err != nil {
				return err
			}
			if result.Deferred {
				if err := renderer.Progress("The replacement will finish after this command exits."); err != nil {
					return fmt.Errorf("render update progress: %w", err)
				}
			}
			status := "updated"
			if dryRun {
				status = "dry-run"
			}
			return renderer.Result(render.ResultRow{
				Action: "update",
				Tool:   "Johan Bostrom CLI",
				Status: status,
			})
		},
	}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "verify the latest release without changing the installed CLI")
	return command
}

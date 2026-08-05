// Package cli defines the Johan Bostrom command-line interface.
package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// Execute runs the CLI with its standard input and output streams.
func Execute(ctx context.Context, args []string) error {
	return ExecuteWithIO(ctx, args, nil, nil)
}

// ExecuteWithIO runs the CLI with injectable output streams.
func ExecuteWithIO(ctx context.Context, args []string, out, errOut io.Writer) error {
	root := newRootCommand()
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errOut)
	return root.ExecuteContext(ctx)
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "jb",
		Short: "Johan Bostrom CLI",
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "tools",
			Short: "Manage tools",
		},
		newVersionCommand(),
	)
	return root
}

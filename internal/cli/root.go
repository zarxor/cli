// Package cli defines the Johan Bostrom command-line interface.
package cli

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/zarxor/cli/internal/install"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/version"
)

type themeFactory func(*cobra.Command) render.Theme

// Execute runs the CLI with its standard input and output streams.
func Execute(ctx context.Context, args []string) error {
	return ExecuteWithIO(ctx, args, nil, nil)
}

// ExecuteWithIO runs the CLI with injectable output streams.
func ExecuteWithIO(ctx context.Context, args []string, out, errOut io.Writer) error {
	root := newRootCommand(newLiveToolsService())
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errOut)
	return root.ExecuteContext(ctx)
}

func newRootCommand(services ...ToolsService) *cobra.Command {
	service := ToolsService(newLiveToolsService())
	if len(services) > 0 {
		service = services[0]
	}
	return newRootCommandWithAllServices(service, newLiveSkillsService(), newLiveServiceService(), func(cmd *cobra.Command) render.Theme {
		return render.AutoTheme(cmd.InOrStdin(), cmd.OutOrStdout(), os.Environ())
	})
}

func newRootCommandWithTheme(service ToolsService, themeFor themeFactory) *cobra.Command {
	return newRootCommandWithServices(service, newLiveSkillsService(), themeFor)
}

func newRootCommandWithServices(service ToolsService, skillsService SkillsService, themeFor themeFactory) *cobra.Command {
	return newRootCommandWithAllServices(service, skillsService, newLiveServiceService(), themeFor)
}

func newRootCommandWithAllServices(service ToolsService, skillsService SkillsService, serviceService ServiceService, themeFor themeFactory) *cobra.Command {
	root := &cobra.Command{
		Use:   "jb",
		Short: "Johan Bostrom CLI",
	}
	root.AddCommand(
		newToolsCommand(service),
		newRootInspectionCommand(service, install.Status),
		newRootInspectionCommand(service, install.Doctor),
		newSkillsCommand(skillsService),
		newServiceCommand(serviceService),
		newUpdateCommand(themeFor),
		newVersionCommand(themeFor),
	)
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		var plain bytes.Buffer
		output := cmd.OutOrStdout()
		cmd.SetOut(&plain)
		defaultHelp(cmd, args)
		cmd.SetOut(output)
		renderer := render.NewRenderer(output, themeFor(cmd))
		_ = renderer.Version("Johan Bostrom CLI", version.Version)
		_ = renderer.Help(plain.String())
	})
	return root
}

// PrintError writes a consistent command failure to the supplied stream.
func PrintError(writer io.Writer, err error) error {
	theme := render.AutoTheme(os.Stdin, writer, os.Environ())
	return printErrorWithTheme(writer, err, theme)
}

func printErrorWithTheme(writer io.Writer, err error, theme render.Theme) error {
	return render.NewRenderer(writer, theme).Error(err)
}

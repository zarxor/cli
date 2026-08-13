package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zarxor/cli/internal/runner"
	backgroundservice "github.com/zarxor/cli/internal/service"
)

// ServiceRequest is the parsed command-layer request passed to ServiceService.
type ServiceRequest struct {
	Action  backgroundservice.Action
	Name    string
	BaseDir string
	DryRun  bool
	Writer  io.Writer
}

// ServiceService keeps service command parsing independent from user
// environment discovery and external command execution.
type ServiceService interface {
	Run(ctx context.Context, request ServiceRequest) (backgroundservice.Result, error)
}

type serviceService struct {
	loadManager func() (*backgroundservice.T3CodeManager, error)
}

func newServiceCommand(service ServiceService) *cobra.Command {
	command := &cobra.Command{
		Use:     "service",
		Aliases: []string{"services"},
		Short:   "Manage background services",
	}
	command.AddCommand(
		newServiceActionCommand(service, backgroundservice.Install),
		newServiceActionCommand(service, backgroundservice.Update),
		newServiceActionCommand(service, backgroundservice.Status),
		newServiceActionCommand(service, backgroundservice.Uninstall),
	)
	return command
}

func newServiceActionCommand(service ServiceService, action backgroundservice.Action) *cobra.Command {
	var baseDir string
	var dryRun bool

	command := &cobra.Command{
		Use:   string(action) + " [t3-code]",
		Short: serviceActionDescription(action),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			name, err := serviceName(args)
			if err != nil {
				return err
			}
			result, err := service.Run(command.Context(), ServiceRequest{
				Action:  action,
				Name:    name,
				BaseDir: baseDir,
				DryRun:  dryRun,
				Writer:  command.OutOrStdout(),
			})
			if err != nil {
				return err
			}
			writer := command.OutOrStdout()
			if result.DryRun {
				if _, err := fmt.Fprintf(writer, "Would run: %s\n", result.Command); err != nil {
					return fmt.Errorf("write service dry-run: %w", err)
				}
			}
			if result.Output != "" {
				if _, err := io.WriteString(writer, result.Output); err != nil {
					return fmt.Errorf("write service output: %w", err)
				}
				if !strings.HasSuffix(result.Output, "\n") {
					if _, err := io.WriteString(writer, "\n"); err != nil {
						return fmt.Errorf("write service output newline: %w", err)
					}
				}
			}
			return nil
		},
	}
	command.Flags().StringVar(&baseDir, "base-dir", "", "T3 Code data directory")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the command without changing the host")
	return command
}

func serviceName(args []string) (string, error) {
	if len(args) == 0 {
		return "t3-code", nil
	}
	name := strings.ToLower(strings.TrimSpace(args[0]))
	switch name {
	case "t3", "t3code", "t3-code":
		return "t3-code", nil
	default:
		return "", fmt.Errorf("unknown service %q (supported service: t3-code)", args[0])
	}
}

func serviceActionDescription(action backgroundservice.Action) string {
	switch action {
	case backgroundservice.Install:
		return "Install T3 Code as an autostarting background service"
	case backgroundservice.Update:
		return "Update or repair the T3 Code background service"
	case backgroundservice.Status:
		return "Show the T3 Code background service status"
	case backgroundservice.Uninstall:
		return "Stop and remove the T3 Code background service"
	default:
		return "Manage a background service"
	}
}

func (s *serviceService) Run(ctx context.Context, request ServiceRequest) (backgroundservice.Result, error) {
	if request.Name != "t3-code" {
		return backgroundservice.Result{}, fmt.Errorf("unsupported service %q", request.Name)
	}
	manager, err := s.loadManager()
	if err != nil {
		return backgroundservice.Result{}, err
	}
	return manager.Run(ctx, request.Action, request.BaseDir, request.DryRun)
}

func newLiveServiceService() ServiceService {
	return &serviceService{loadManager: loadLiveServiceManager}
}

func loadLiveServiceManager() (*backgroundservice.T3CodeManager, error) {
	if runtime.GOOS != "linux" {
		home, _ := os.UserHomeDir()
		return backgroundservice.NewT3CodeManager(runner.NewExec(), backgroundservice.Config{
			Platform: runtime.GOOS,
			Home:     home,
		}), nil
	}

	config, err := liveLinuxConfig()
	if err != nil {
		return nil, fmt.Errorf("resolve T3 Code service user: %w", err)
	}
	return backgroundservice.NewT3CodeManager(runner.NewExec(), backgroundservice.Config{
		Platform:     runtime.GOOS,
		Home:         config.Home,
		Root:         config.Root,
		InvokingUser: config.InvokingUser,
		InvokingUID:  config.InvokingUID,
	}), nil
}

var _ ServiceService = (*serviceService)(nil)

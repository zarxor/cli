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
	Lines   int
	Follow  bool
	Writer  io.Writer
}

// ServiceService keeps service command parsing independent from user
// environment discovery and external command execution.
type ServiceService interface {
	Run(ctx context.Context, request ServiceRequest) (backgroundservice.Result, error)
}

type serviceService struct {
	providers map[string]serviceProvider
}

type serviceProvider struct {
	name        string
	aliases     []string
	loadManager func() (backgroundservice.Manager, error)
}

func liveServiceProviders() []serviceProvider {
	return []serviceProvider{
		{
			name:        "t3-code",
			aliases:     []string{"t3", "t3code"},
			loadManager: loadLiveServiceManager,
		},
	}
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
		newServiceActionCommand(service, backgroundservice.Start),
		newServiceActionCommand(service, backgroundservice.Stop),
		newServiceActionCommand(service, backgroundservice.Restart),
		newServiceActionCommand(service, backgroundservice.Logs),
		newServiceActionCommand(service, backgroundservice.Repair),
	)
	return command
}

func newServiceActionCommand(service ServiceService, action backgroundservice.Action) *cobra.Command {
	var baseDir string
	var dryRun bool
	var lines int
	var follow bool

	command := &cobra.Command{
		Use:   string(action) + " [service]",
		Short: serviceActionDescription(action),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			name, err := serviceName(args, liveServiceProviders())
			if err != nil {
				return err
			}
			result, err := service.Run(command.Context(), ServiceRequest{
				Action:  action,
				Name:    name,
				BaseDir: baseDir,
				DryRun:  dryRun,
				Lines:   lines,
				Follow:  follow,
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
	command.Flags().StringVar(&baseDir, "base-dir", "", "service data directory")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the command without changing the host")
	if action == backgroundservice.Logs {
		command.Flags().IntVar(&lines, "lines", 100, "number of recent log lines to show")
		command.Flags().BoolVar(&follow, "follow", false, "follow service logs")
	}
	return command
}

func serviceName(args []string, providers []serviceProvider) (string, error) {
	if len(providers) == 0 {
		return "", fmt.Errorf("no background services are registered")
	}
	if len(args) == 0 {
		return providers[0].name, nil
	}
	name := strings.ToLower(strings.TrimSpace(args[0]))
	available := make([]string, 0, len(providers))
	for _, provider := range providers {
		available = append(available, provider.name)
		if name == provider.name {
			return provider.name, nil
		}
		for _, alias := range provider.aliases {
			if name == alias {
				return provider.name, nil
			}
		}
	}
	return "", fmt.Errorf("unknown service %q (supported services: %s)", args[0], strings.Join(available, ", "))
}

func serviceActionDescription(action backgroundservice.Action) string {
	switch action {
	case backgroundservice.Install:
		return "Install an autostarting background service"
	case backgroundservice.Update:
		return "Update a background service"
	case backgroundservice.Status:
		return "Show a background service status"
	case backgroundservice.Uninstall:
		return "Stop and remove a background service"
	case backgroundservice.Start:
		return "Start a background service"
	case backgroundservice.Stop:
		return "Stop a background service"
	case backgroundservice.Restart:
		return "Restart a background service"
	case backgroundservice.Logs:
		return "Show background service logs"
	case backgroundservice.Repair:
		return "Repair a background service"
	default:
		return "Manage a background service"
	}
}

func (s *serviceService) Run(ctx context.Context, request ServiceRequest) (backgroundservice.Result, error) {
	provider, ok := s.providers[request.Name]
	if !ok {
		return backgroundservice.Result{}, fmt.Errorf("unsupported service %q", request.Name)
	}
	manager, err := provider.loadManager()
	if err != nil {
		return backgroundservice.Result{}, err
	}
	return manager.RunWithOptions(ctx, request.Action, request.BaseDir, request.DryRun, backgroundservice.RunOptions{
		Lines: request.Lines, Follow: request.Follow,
	})
}

func newLiveServiceService() ServiceService {
	providers := make(map[string]serviceProvider)
	for _, provider := range liveServiceProviders() {
		providers[provider.name] = provider
	}
	return &serviceService{providers: providers}
}

func loadLiveServiceManager() (backgroundservice.Manager, error) {
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

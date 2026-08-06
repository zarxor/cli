package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zarxor/scripts/internal/adapters"
	"github.com/zarxor/scripts/internal/install"
	"github.com/zarxor/scripts/internal/plan"
	"github.com/zarxor/scripts/internal/platform"
	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/render"
	"github.com/zarxor/scripts/internal/runner"
	"github.com/zarxor/scripts/internal/tools"
)

// ToolsRequest is the parsed command-layer request passed to ToolsService.
type ToolsRequest struct {
	Action       install.Action
	ProfileNames []string
	Only         []tools.ToolID
	Yes          bool
	DryRun       bool
	Writer       io.Writer
	Selection    install.SelectionUI
}

// ToolsService keeps command parsing independent from host detection and
// package-manager activity.
type ToolsService interface {
	Run(ctx context.Context, request ToolsRequest) error
}

type toolsService struct {
	loadAdapter func() (adapters.Adapter, error)
}

func newToolsCommand(service ToolsService) *cobra.Command {
	command := &cobra.Command{
		Use:   "tools",
		Short: "Manage tools",
	}
	command.AddCommand(
		newToolsActionCommand(service, install.Install),
		newToolsActionCommand(service, install.Update),
	)
	return command
}

func newToolsActionCommand(service ToolsService, action install.Action) *cobra.Command {
	var profilesValue string
	var onlyValue string
	var yes bool
	var dryRun bool

	command := &cobra.Command{
		Use:   string(action),
		Short: titleAction(action) + " tools",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			profileNames, err := commaSeparated(command, "profiles", profilesValue, "profile name")
			if err != nil {
				return err
			}
			onlyNames, err := commaSeparated(command, "only", onlyValue, "tool name")
			if err != nil {
				return err
			}
			only := make([]tools.ToolID, len(onlyNames))
			for i, name := range onlyNames {
				only[i] = tools.ToolID(name)
			}
			input := command.InOrStdin()
			writer := command.OutOrStdout()
			theme := render.AutoTheme(input, writer, os.Environ())
			return service.Run(command.Context(), ToolsRequest{
				Action:       action,
				ProfileNames: profileNames,
				Only:         only,
				Yes:          yes,
				DryRun:       dryRun,
				Writer:       writer,
				Selection:    render.NewAdaptiveSelection(input, writer, theme),
			})
		},
	}
	command.Flags().StringVar(&profilesValue, "profiles", "", "comma-separated profile names")
	command.Flags().StringVar(&onlyValue, "only", "", "comma-separated tool names")
	command.Flags().BoolVar(&yes, "yes", false, "select all planned tools without prompting")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan without changing the host")
	return command
}

func commaSeparated(command *cobra.Command, flagName, value, itemName string) ([]string, error) {
	if !command.Flags().Changed(flagName) {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, len(parts))
	for i, part := range parts {
		values[i] = strings.TrimSpace(part)
		if values[i] == "" {
			return nil, fmt.Errorf("%s cannot be empty", itemName)
		}
	}
	return values, nil
}

func titleAction(action install.Action) string {
	if action == install.Install {
		return "Install"
	}
	return "Update"
}

func (s *toolsService) Run(ctx context.Context, request ToolsRequest) error {
	profiles, err := profile.ResolveProfiles(request.ProfileNames)
	if err != nil {
		return err
	}

	planned, err := requestedTools(request.Action, profiles, request.Only)
	if err != nil {
		return err
	}
	adapter, err := s.loadAdapter()
	if err != nil {
		return err
	}

	statuses := make([]install.ToolStatus, 0, len(planned))
	for _, tool := range planned {
		detection, detectErr := adapter.Detect(ctx, tool)
		if detectErr != nil {
			return fmt.Errorf("detect %s: %w", tool.Name, detectErr)
		}
		statuses = append(statuses, install.ToolStatus{
			Tool:             tool,
			Installed:        detection.Installed,
			Selected:         true,
			CurrentVersion:   detection.Current,
			CandidateVersion: detection.Candidate,
		})
	}

	// Run accepts an adapter set keyed by its own live platform detection.
	// Repeating the already-selected adapter keeps injected fixture services
	// portable while the live loader remains the sole native/WSL routing point.
	adapterSet := map[platform.OS]adapters.Adapter{
		platform.Debian:  adapter,
		platform.Arch:    adapter,
		platform.Windows: adapter,
	}
	summary := install.Run(ctx, request.Action, statuses, adapterSet, install.Options{
		Yes:       request.Yes,
		DryRun:    request.DryRun,
		Writer:    request.Writer,
		Selection: request.Selection,
	})
	if !summary.Failed {
		return nil
	}
	for _, result := range summary.Results {
		if result.Err != nil {
			return fmt.Errorf("%s %s failed: %w", result.Action, result.Tool.ID, result.Err)
		}
	}
	return fmt.Errorf("tools %s failed", request.Action)
}

func requestedTools(action install.Action, profiles []profile.Profile, only []tools.ToolID) ([]tools.Tool, error) {
	if len(profiles) == 0 && len(only) == 0 {
		return append([]tools.Tool(nil), tools.Catalog...), nil
	}
	if action == install.Update && len(profiles) == 0 {
		return tools.ResolveTools(only)
	}
	return plan.MergeProfiles(profiles, only)
}

func newLiveToolsService() ToolsService {
	return &toolsService{loadAdapter: loadLiveAdapter}
}

func loadLiveAdapter() (adapters.Adapter, error) {
	host, err := platform.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect platform: %w", err)
	}
	commandRunner := runner.NewExec()
	if host == platform.Windows {
		return adapters.NewWindowsAdapter(commandRunner, nil), nil
	}

	config, err := liveLinuxConfig()
	if err != nil {
		return nil, err
	}
	switch host {
	case platform.Debian:
		return adapters.NewDebianAdapter(commandRunner, nil, config), nil
	case platform.Arch:
		return adapters.NewArchAdapter(commandRunner, nil, config), nil
	default:
		return nil, fmt.Errorf("unsupported platform %q", host)
	}
}

func liveLinuxConfig() (adapters.LinuxConfig, error) {
	release, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return adapters.LinuxConfig{}, fmt.Errorf("read /etc/os-release: %w", err)
	}
	currentUser, err := user.Current()
	if err != nil {
		return adapters.LinuxConfig{}, fmt.Errorf("find invoking user: %w", err)
	}
	return linuxConfigFrom(string(release), runtime.GOARCH, currentUser, os.Getenv("SUDO_USER"), user.Lookup)
}

func linuxConfigFrom(release, goarch string, currentUser *user.User, sudoUser string, lookup func(string) (*user.User, error)) (adapters.LinuxConfig, error) {
	metadata := releaseMetadata(release)
	architecture, err := linuxArchitecture(goarch)
	if err != nil {
		return adapters.LinuxConfig{}, err
	}
	invokingUser := currentUser
	root := currentUser.Uid == "0"
	if root && sudoUser != "" {
		invokingUser, err = lookup(sudoUser)
		if err != nil {
			return adapters.LinuxConfig{}, fmt.Errorf("find invoking user %q: %w", sudoUser, err)
		}
	}
	if invokingUser.HomeDir == "" {
		return adapters.LinuxConfig{}, fmt.Errorf("invoking user home is empty")
	}
	uid, err := strconv.Atoi(invokingUser.Uid)
	if err != nil {
		return adapters.LinuxConfig{}, fmt.Errorf("parse invoking user UID %q: %w", invokingUser.Uid, err)
	}
	gid, err := strconv.Atoi(invokingUser.Gid)
	if err != nil {
		return adapters.LinuxConfig{}, fmt.Errorf("parse invoking user GID %q: %w", invokingUser.Gid, err)
	}
	codename := metadata["VERSION_CODENAME"]
	if codename == "" {
		codename = metadata["UBUNTU_CODENAME"]
	}
	return adapters.LinuxConfig{
		Root:         root,
		Home:         invokingUser.HomeDir,
		InvokingUser: invokingUser.Username,
		InvokingUID:  uid,
		InvokingGID:  gid,
		Distribution: metadata["ID"],
		Codename:     codename,
		Architecture: architecture,
	}, nil
}

func releaseMetadata(release string) map[string]string {
	metadata := make(map[string]string)
	for _, line := range strings.Split(release, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		metadata[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
	}
	return metadata
}

func linuxArchitecture(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported Linux architecture %q", goarch)
	}
}

var _ ToolsService = (*toolsService)(nil)

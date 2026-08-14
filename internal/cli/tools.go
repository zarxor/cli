package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/zarxor/cli/internal/adapters"
	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/host"
	"github.com/zarxor/cli/internal/install"
	"github.com/zarxor/cli/internal/plan"
	"github.com/zarxor/cli/internal/platform"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/runner"
	"github.com/zarxor/cli/internal/tools"
)

// ToolsRequest is the parsed command-layer request passed to ToolsService.
type ToolsRequest struct {
	Action       install.Action
	ProfileNames []string
	Only         []tools.ToolID
	Yes          bool
	DryRun       bool
	JSON         bool
	Writer       io.Writer
	Renderer     *render.Renderer
	Selection    install.SelectionUI
}

// ToolsService keeps command parsing independent from host detection and
// package-manager activity.
type ToolsService interface {
	Run(ctx context.Context, request ToolsRequest) error
}

type toolsService struct {
	loadAdapter    func() (adapters.Adapter, error)
	detectHost     func() (host.Detection, error)
	detectPlatform func() (platform.OS, error)
}

func newToolsCommand(service ToolsService) *cobra.Command {
	command := &cobra.Command{
		Use:   "tools",
		Short: "Manage tools",
	}
	command.AddCommand(
		newToolsActionCommand(service, install.Install),
		newToolsActionCommand(service, install.Update),
		newToolsActionCommand(service, install.Repair),
		newToolsActionCommand(service, install.List),
		newToolsActionCommand(service, install.Outdated),
	)
	return command
}

func newToolsActionCommand(service ToolsService, action install.Action) *cobra.Command {
	var profilesValue string
	var onlyValue string
	var yes bool
	var dryRun bool
	var jsonOutput bool

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
			renderer := render.NewRenderer(writer, theme)
			return service.Run(command.Context(), ToolsRequest{
				Action:       action,
				ProfileNames: profileNames,
				Only:         only,
				Yes:          yes,
				DryRun:       dryRun,
				JSON:         jsonOutput,
				Writer:       writer,
				Renderer:     renderer,
				Selection:    render.NewAdaptiveSelection(input, writer, theme),
			})
		},
	}
	command.Flags().StringVar(&profilesValue, "profiles", "", "comma-separated profile names")
	command.Flags().StringVar(&onlyValue, "only", "", "comma-separated tool names")
	if action == install.Install || action == install.Update || action == install.Repair {
		command.Flags().BoolVar(&yes, "yes", false, "select all planned tools without prompting")
		command.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan without changing the host")
	}
	if action == install.List || action == install.Outdated {
		command.Flags().BoolVar(&jsonOutput, "json", false, "write machine-readable JSON")
	}
	return command
}

func newRootInspectionCommand(service ToolsService, action install.Action) *cobra.Command {
	var profilesValue string
	var onlyValue string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   string(action),
		Short: titleAction(action) + " host and tool state",
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
			writer := command.OutOrStdout()
			return service.Run(command.Context(), ToolsRequest{
				Action:       action,
				ProfileNames: profileNames,
				Only:         only,
				JSON:         jsonOutput,
				Writer:       writer,
				Renderer:     render.NewRenderer(writer, render.AutoTheme(command.InOrStdin(), writer, os.Environ())),
			})
		},
	}
	command.Flags().StringVar(&profilesValue, "profiles", "", "comma-separated profile names")
	command.Flags().StringVar(&onlyValue, "only", "", "comma-separated tool names")
	command.Flags().BoolVar(&jsonOutput, "json", false, "write machine-readable JSON")
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
	switch action {
	case install.Install:
		return "Install"
	case install.Update:
		return "Update"
	case install.Repair:
		return "Repair"
	case install.List:
		return "List"
	case install.Outdated:
		return "Show outdated"
	case install.Status:
		return "Show status"
	case install.Doctor:
		return "Check"
	}
	return string(action)
}

func (s *toolsService) Run(ctx context.Context, request ToolsRequest) error {
	if request.Action == install.List || request.Action == install.Outdated || request.Action == install.Status || request.Action == install.Doctor {
		return s.runInspection(ctx, request)
	}
	profiles, appliedProfiles, detection, err := s.resolveProfiles(request)
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

	renderer := request.Renderer
	if renderer == nil {
		renderer = render.NewPlainRenderer(request.Writer)
	}
	if err := renderAppliedProfiles(renderer, appliedProfiles, detection, len(request.Only) > 0 && len(appliedProfiles) == 0); err != nil {
		return fmt.Errorf("render applied profiles: %w", err)
	}
	if err := renderer.ProgressBar("Checking installed tools", 0, len(planned)); err != nil {
		return fmt.Errorf("render discovery progress: %w", err)
	}
	statuses, err := detectTools(ctx, adapter, planned, renderer)
	if err != nil {
		return err
	}

	// Run accepts an adapter set keyed by its own live platform detection.
	// Repeating the already-selected adapter keeps injected fixture services
	// portable while the live loader remains the sole native/WSL routing point.
	adapterSet := map[platform.OS]adapters.Adapter{
		platform.Debian:  adapter,
		platform.Arch:    adapter,
		platform.Windows: adapter,
		platform.Darwin:  adapter,
	}
	summary := install.Run(ctx, request.Action, statuses, adapterSet, install.Options{
		Yes:       request.Yes,
		DryRun:    request.DryRun,
		Writer:    request.Writer,
		Renderer:  renderer,
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

type inspectionReport struct {
	Action     string           `json:"action"`
	Platform   string           `json:"platform"`
	Profiles   []string         `json:"profiles"`
	HostRole   string           `json:"host_role,omitempty"`
	HostReason string           `json:"host_reason,omitempty"`
	Healthy    bool             `json:"healthy"`
	Tools      []inspectionJSON `json:"tools"`
	Issues     []string         `json:"issues,omitempty"`
}

type inspectionJSON struct {
	ID        tools.ToolID `json:"id"`
	Name      string       `json:"name"`
	State     string       `json:"state"`
	Installed bool         `json:"installed"`
	Current   string       `json:"current,omitempty"`
	Candidate string       `json:"candidate,omitempty"`
}

func (s *toolsService) runInspection(ctx context.Context, request ToolsRequest) error {
	profiles, appliedProfiles, hostDetection, err := s.resolveInspectionProfiles(request)
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
	platformName, err := s.currentPlatform()
	if err != nil {
		return err
	}
	renderer := request.Renderer
	if renderer == nil {
		renderer = render.NewPlainRenderer(request.Writer)
	}
	discoveryRenderer := renderer
	if request.JSON {
		discoveryRenderer = render.NewPlainRenderer(io.Discard)
	}
	if !request.JSON {
		if err := renderAppliedProfiles(discoveryRenderer, appliedProfiles, hostDetection, false); err != nil {
			return fmt.Errorf("render applied profiles: %w", err)
		}
		if err := discoveryRenderer.Progress("Platform: " + string(platformName)); err != nil {
			return fmt.Errorf("render platform: %w", err)
		}
		if err := discoveryRenderer.ProgressBar("Checking installed tools", 0, len(planned)); err != nil {
			return fmt.Errorf("render discovery progress: %w", err)
		}
	} else if len(planned) == 0 {
		// Keep the JSON path deterministic without asking the renderer to draw a
		// progress bar for an empty selection.
	}
	statuses, err := detectTools(ctx, adapter, planned, discoveryRenderer)
	if err != nil {
		return err
	}
	if request.Action == install.Outdated {
		filtered := statuses[:0]
		for _, status := range statuses {
			if install.IsOutdated(status) {
				filtered = append(filtered, status)
			}
		}
		statuses = filtered
	}
	report := makeInspectionReport(request.Action, platformName, appliedProfiles, hostDetection, statuses)
	if request.JSON {
		if err := writeInspectionJSON(request.Writer, report); err != nil {
			return err
		}
		if request.Action == install.Doctor && !report.Healthy {
			return fmt.Errorf("doctor found %d issue(s)", len(report.Issues))
		}
		return nil
	}
	if err := renderer.StatusTable(inspectionRows(statuses)); err != nil {
		return fmt.Errorf("render tool status: %w", err)
	}
	if len(statuses) == 0 {
		message := "No outdated tools found."
		if request.Action != install.Outdated {
			message = "No tools selected."
		}
		if err := renderer.Progress(message); err != nil {
			return err
		}
	}
	for _, issue := range report.Issues {
		if err := renderer.Progress("Issue: " + issue); err != nil {
			return err
		}
	}
	if request.Action == install.Doctor && !report.Healthy {
		return fmt.Errorf("doctor found %d issue(s)", len(report.Issues))
	}
	return nil
}

func (s *toolsService) resolveInspectionProfiles(request ToolsRequest) ([]profile.Profile, []profile.ProfileName, *host.Detection, error) {
	if len(request.ProfileNames) > 0 || len(request.Only) > 0 {
		return s.resolveProfiles(request)
	}
	if request.Action == install.List || request.Action == install.Outdated {
		return nil, nil, nil, nil
	}
	return s.resolveProfiles(request)
}

func (s *toolsService) currentPlatform() (platform.OS, error) {
	detectPlatform := s.detectPlatform
	if detectPlatform == nil {
		detectPlatform = platform.Detect
	}
	name, err := detectPlatform()
	if err != nil {
		return "", fmt.Errorf("detect platform: %w", err)
	}
	return name, nil
}

func makeInspectionReport(action install.Action, platformName platform.OS, profiles []profile.ProfileName, hostDetection *host.Detection, statuses []install.ToolStatus) inspectionReport {
	report := inspectionReport{
		Action:   string(action),
		Platform: string(platformName),
		Healthy:  true,
		Profiles: make([]string, 0, len(profiles)),
		Tools:    make([]inspectionJSON, 0, len(statuses)),
	}
	for _, name := range profiles {
		report.Profiles = append(report.Profiles, string(name))
	}
	if hostDetection != nil {
		report.HostRole = string(hostDetection.Role)
		report.HostReason = hostDetection.Reason
	}
	for _, status := range statuses {
		state := inspectionState(status)
		if action == install.Doctor && state == "missing" {
			report.Healthy = false
			report.Issues = append(report.Issues, status.Tool.Name+" is not installed")
		}
		if action == install.Doctor && state == "outdated" {
			report.Healthy = false
			report.Issues = append(report.Issues, status.Tool.Name+" has an available update")
		}
		report.Tools = append(report.Tools, inspectionJSON{
			ID: status.Tool.ID, Name: status.Tool.Name, State: state,
			Installed: status.Installed, Current: status.CurrentVersion, Candidate: status.CandidateVersion,
		})
	}
	return report
}

func inspectionRows(statuses []install.ToolStatus) []render.StatusRow {
	rows := make([]render.StatusRow, 0, len(statuses))
	for _, status := range statuses {
		rows = append(rows, render.StatusRow{Tool: status.Tool, State: inspectionState(status), CurrentVersion: status.CurrentVersion, CandidateVersion: status.CandidateVersion})
	}
	return rows
}

func inspectionState(status install.ToolStatus) string {
	if !status.Installed {
		return "missing"
	}
	if install.IsOutdated(status) {
		return "outdated"
	}
	return "installed"
}

func writeInspectionJSON(writer io.Writer, report inspectionReport) error {
	if writer == nil {
		writer = io.Discard
	}
	if err := json.NewEncoder(writer).Encode(report); err != nil {
		return fmt.Errorf("write JSON status: %w", err)
	}
	return nil
}

func (s *toolsService) resolveProfiles(request ToolsRequest) ([]profile.Profile, []profile.ProfileName, *host.Detection, error) {
	if len(request.ProfileNames) > 0 {
		profiles, err := profile.ResolveProfiles(request.ProfileNames)
		if err != nil {
			return nil, nil, nil, err
		}
		applied := make([]profile.ProfileName, 0, len(request.ProfileNames))
		for _, name := range request.ProfileNames {
			applied = append(applied, profile.ProfileName(name))
		}
		return profiles, applied, nil, nil
	}
	if len(request.Only) > 0 {
		return nil, nil, nil, nil
	}

	detectHost := s.detectHost
	if detectHost == nil {
		detectHost = host.Detect
	}
	detection, err := detectHost()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("detect host role: %w", err)
	}
	profileName := profile.Desktop
	if detection.Role == host.Server {
		profileName = profile.Server
	}
	profiles, err := profile.ResolveProfiles([]string{string(profileName)})
	if err != nil {
		return nil, nil, nil, err
	}
	return profiles, []profile.ProfileName{profileName}, &detection, nil
}

func renderAppliedProfiles(renderer *render.Renderer, profiles []profile.ProfileName, detection *host.Detection, explicitTools bool) error {
	message := "Applied profiles: none"
	if len(profiles) > 0 {
		names := make([]string, 0, len(profiles))
		for _, name := range profiles {
			names = append(names, string(name))
		}
		message = "Applied profiles: " + strings.Join(names, ", ")
		if detection != nil {
			message += " (auto-detected " + string(detection.Role) + ": " + detection.Reason + ")"
		}
	} else if explicitTools {
		message += " (explicit tool selection)"
	}
	return renderer.Progress(message)
}

// Detection is mostly command and network I/O. Keep it bounded so a full
// catalog starts quickly without launching an unbounded provider fan-out.
const detectionWorkers = 4

type detectionResult struct {
	index     int
	detection detect.Detection
	err       error
}

func detectTools(ctx context.Context, adapter adapters.Adapter, planned []tools.Tool, renderer *render.Renderer) ([]install.ToolStatus, error) {
	if len(planned) == 0 {
		return nil, nil
	}

	scanContext, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	results := make(chan detectionResult, len(planned))
	workerCount := len(planned)
	if workerCount > detectionWorkers {
		workerCount = detectionWorkers
	}

	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				detection, err := adapter.Detect(scanContext, planned[index])
				results <- detectionResult{index: index, detection: detection, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range planned {
			select {
			case jobs <- index:
			case <-scanContext.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	statuses := make([]install.ToolStatus, len(planned))
	completed := 0
	var firstErr error
	for result := range results {
		completed++
		if result.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("detect %s: %w", planned[result.index].Name, result.err)
				cancel()
			}
			continue
		}
		tool := planned[result.index]
		statuses[result.index] = install.ToolStatus{
			Tool:             tool,
			Installed:        result.detection.Installed,
			Selected:         true,
			CurrentVersion:   result.detection.Current,
			CandidateVersion: result.detection.Candidate,
		}
		if err := renderer.ProgressBar("Checking installed tools", completed, len(planned)); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("render discovery progress: %w", err)
			cancel()
		}
	}
	if firstErr != nil {
		_ = renderer.FinishProgress()
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		_ = renderer.FinishProgress()
		return nil, err
	}
	return statuses, nil
}

func requestedTools(action install.Action, profiles []profile.Profile, only []tools.ToolID) ([]tools.Tool, error) {
	if len(profiles) == 0 && len(only) == 0 {
		return append([]tools.Tool(nil), tools.Catalog...), nil
	}
	if (action == install.Update || action == install.Repair || action == install.List || action == install.Outdated) && len(profiles) == 0 {
		return tools.ResolveTools(only)
	}
	return plan.MergeProfiles(profiles, only)
}

func newLiveToolsService() ToolsService {
	return &toolsService{loadAdapter: loadLiveAdapter, detectHost: host.Detect, detectPlatform: platform.Detect}
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
	if host == platform.Darwin {
		return adapters.NewDarwinAdapter(commandRunner, nil), nil
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

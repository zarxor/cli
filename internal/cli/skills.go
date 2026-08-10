package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/skills"
)

// SkillsAction is the command-layer action for Agent Skill lifecycle operations.
type SkillsAction string

const (
	SkillsInstall SkillsAction = "install"
	SkillsList    SkillsAction = "list"
	SkillsInfo    SkillsAction = "info"
	SkillsUpdate  SkillsAction = "update"
	SkillsRemove  SkillsAction = "remove"
	SkillsVerify  SkillsAction = "verify"
	SkillsDoctor  SkillsAction = "doctor"
)

// SkillsRequest is the parsed command-layer request passed to SkillsService.
type SkillsRequest struct {
	Action           SkillsAction
	Only             []skills.SkillID
	Names            []string
	Input            io.Reader
	ScopeMode        skills.ScopeMode
	ScopeSet         bool
	Harnesses        []skills.Target
	HarnessesSet     bool
	Target           skills.Target
	Scope            skills.Scope
	DryRun           bool
	Yes              bool
	Writer           io.Writer
	Renderer         *render.Renderer
	Selection        render.SelectionUI
	HarnessSelection render.SelectionUI
}

// SkillsService keeps command parsing independent from filesystem and network activity.
type SkillsService interface {
	Run(ctx context.Context, request SkillsRequest) error
}

type skillsService struct {
	manager *skills.Manager
	err     error
}

func newLiveSkillsService() SkillsService {
	manager, err := skills.NewManager(skills.Environment{})
	return &skillsService{manager: manager, err: err}
}

func newSkillsCommand(service SkillsService) *cobra.Command {
	command := &cobra.Command{
		Use:   "skills",
		Short: "Manage Agent Skills",
	}
	command.AddCommand(
		newSkillsInstallCommand(service),
		newSkillsListCommand(service),
		newSkillsInfoCommand(service),
		newSkillsUpdateCommand(service),
		newSkillsRemoveCommand(service),
		newSkillsVerifyCommand(service),
		newSkillsDoctorCommand(service),
	)
	return command
}

func newSkillsInstallCommand(service SkillsService) *cobra.Command {
	var onlyValue string
	var dryRun bool
	var yes bool
	var scopeValue string
	var harnessValue string
	command := &cobra.Command{
		Use:   "install",
		Short: "Install selected skills from the available catalog",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			onlyNames, err := commaSeparated(command, "only", onlyValue, "skill name")
			if err != nil {
				return err
			}
			scopeMode, err := parseSkillScopeMode(scopeValue)
			if err != nil {
				return err
			}
			harnesses, err := parseSkillHarnesses(harnessValue)
			if err != nil {
				return err
			}
			harnessesSet := command.Flags().Changed("harnesses")
			only := make([]skills.SkillID, len(onlyNames))
			for i, name := range onlyNames {
				only[i] = skills.SkillID(name)
			}
			input := command.InOrStdin()
			writer := command.OutOrStdout()
			theme := render.AutoTheme(input, writer, os.Environ())
			return runSkillsService(command, service, SkillsRequest{
				Action:           SkillsInstall,
				Only:             only,
				Input:            input,
				ScopeMode:        scopeMode,
				ScopeSet:         command.Flags().Changed("scope"),
				Harnesses:        harnesses,
				HarnessesSet:     harnessesSet,
				DryRun:           dryRun,
				Yes:              yes,
				Writer:           writer,
				Renderer:         render.NewRenderer(writer, theme),
				Selection:        render.NewAdaptiveSelectionWithTitle(input, writer, theme, "skills"),
				HarnessSelection: render.NewAdaptiveSelectionWithTitle(input, writer, theme, "harnesses"),
			})
		},
	}
	command.Flags().StringVar(&onlyValue, "only", "", "comma-separated skill names")
	command.Flags().StringVar(&scopeValue, "scope", string(skills.ScopeModeGlobal), "installation scope: global or project; omit to choose interactively")
	command.Flags().StringVar(&harnessValue, "harnesses", "codex,claude", "comma-separated harnesses: codex and claude; both selected by default when prompted")
	command.Flags().BoolVar(&yes, "yes", false, "select all available skills without prompting")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan without changing files")
	return command
}

func newSkillsListCommand(service SkillsService) *cobra.Command {
	var targetValue string
	var scopeValue string
	command := &cobra.Command{
		Use:   "list",
		Short: "List installed Agent Skills",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			target, scope, err := parseSkillTargetScope(targetValue, scopeValue)
			if err != nil {
				return err
			}
			return runSkillsService(command, service, SkillsRequest{Action: SkillsList, Target: target, Scope: scope, Writer: command.OutOrStdout()})
		},
	}
	addSkillTargetScopeFlags(command, &targetValue, &scopeValue)
	return command
}

func newSkillsInfoCommand(service SkillsService) *cobra.Command {
	var targetValue string
	var scopeValue string
	command := &cobra.Command{
		Use:   "info <name>",
		Short: "Show Agent Skill metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			target, scope, err := parseSkillTargetScope(targetValue, scopeValue)
			if err != nil {
				return err
			}
			return runSkillsService(command, service, SkillsRequest{Action: SkillsInfo, Names: args, Target: target, Scope: scope, Writer: command.OutOrStdout()})
		},
	}
	addSkillTargetScopeFlags(command, &targetValue, &scopeValue)
	return command
}

func newSkillsUpdateCommand(service SkillsService) *cobra.Command {
	var onlyValue string
	var dryRun bool
	var yes bool
	var harnessValue string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update selected skills with newer catalog content",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			onlyNames, err := commaSeparated(command, "only", onlyValue, "skill name")
			if err != nil {
				return err
			}
			only := make([]skills.SkillID, len(onlyNames))
			for i, name := range onlyNames {
				only[i] = skills.SkillID(name)
			}
			harnesses, err := parseSkillHarnesses(harnessValue)
			if err != nil {
				return err
			}
			harnessesSet := command.Flags().Changed("harnesses")
			input := command.InOrStdin()
			writer := command.OutOrStdout()
			theme := render.AutoTheme(input, writer, os.Environ())
			return runSkillsService(command, service, SkillsRequest{
				Action:       SkillsUpdate,
				Only:         only,
				Input:        input,
				Harnesses:    harnesses,
				HarnessesSet: harnessesSet,
				DryRun:       dryRun,
				Yes:          yes,
				Writer:       writer,
				Renderer:     render.NewRenderer(writer, theme),
				Selection:    render.NewAdaptiveSelectionWithTitle(input, writer, theme, "skills"),
			})
		},
	}
	command.Flags().StringVar(&onlyValue, "only", "", "comma-separated skill names")
	command.Flags().StringVar(&harnessValue, "harnesses", "codex,claude", "comma-separated harnesses: codex and claude; both selected by default when prompted")
	command.Flags().BoolVar(&yes, "yes", false, "select all available updates without prompting")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show available updates without changing files")
	return command
}

func newSkillsRemoveCommand(service SkillsService) *cobra.Command {
	var targetValue string
	var scopeValue string
	var dryRun bool
	var yes bool
	command := &cobra.Command{
		Use:   "remove <name...>",
		Short: "Remove managed Agent Skills",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes && !dryRun {
				return fmt.Errorf("refusing to remove skills without --yes")
			}
			target, scope, err := parseSkillTargetScope(targetValue, scopeValue)
			if err != nil {
				return err
			}
			return runSkillsService(command, service, SkillsRequest{Action: SkillsRemove, Names: args, Target: target, Scope: scope, DryRun: dryRun, Yes: yes, Writer: command.OutOrStdout(), Renderer: newSkillRenderer(command)})
		},
	}
	addSkillTargetScopeFlags(command, &targetValue, &scopeValue)
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the removal without changing files")
	command.Flags().BoolVar(&yes, "yes", false, "confirm removal of the named managed skills")
	return command
}

func newSkillsVerifyCommand(service SkillsService) *cobra.Command {
	var targetValue string
	var scopeValue string
	command := &cobra.Command{
		Use:   "verify [name...]",
		Short: "Verify Agent Skill metadata and files",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			target, scope, err := parseSkillTargetScope(targetValue, scopeValue)
			if err != nil {
				return err
			}
			return runSkillsService(command, service, SkillsRequest{Action: SkillsVerify, Names: args, Target: target, Scope: scope, Writer: command.OutOrStdout()})
		},
	}
	addSkillTargetScopeFlags(command, &targetValue, &scopeValue)
	return command
}

func newSkillsDoctorCommand(service SkillsService) *cobra.Command {
	var targetValue string
	var scopeValue string
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose Agent Skill targets and installations",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			target, scope, err := parseSkillTargetScope(targetValue, scopeValue)
			if err != nil {
				return err
			}
			return runSkillsService(command, service, SkillsRequest{Action: SkillsDoctor, Target: target, Scope: scope, Writer: command.OutOrStdout()})
		},
	}
	addSkillTargetScopeFlags(command, &targetValue, &scopeValue)
	return command
}

func addSkillTargetScopeFlags(command *cobra.Command, targetValue, scopeValue *string) {
	command.Flags().StringVar(targetValue, "target", string(skills.TargetCodex), "agent target: codex, claude, copilot, agents, or all")
	command.Flags().StringVar(scopeValue, "scope", string(skills.ScopeUser), "installation scope: user or project")
}

func parseSkillTargetScope(targetValue, scopeValue string) (skills.Target, skills.Scope, error) {
	target, err := skills.ParseTarget(strings.ToLower(strings.TrimSpace(targetValue)))
	if err != nil {
		return "", "", err
	}
	scope, err := skills.ParseScope(strings.ToLower(strings.TrimSpace(scopeValue)))
	if err != nil {
		return "", "", err
	}
	return target, scope, nil
}

func parseSkillScopeMode(value string) (skills.ScopeMode, error) {
	return skills.ParseScopeMode(strings.ToLower(strings.TrimSpace(value)))
}

func parseSkillHarnesses(value string) ([]skills.Target, error) {
	parts := strings.Split(value, ",")
	result := make([]skills.Target, 0, len(parts))
	for _, part := range parts {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			return nil, fmt.Errorf("harness name cannot be empty")
		}
		target, err := skills.ParseTarget(name)
		if err != nil {
			return nil, fmt.Errorf("unknown skill harness %q (want codex or claude)", name)
		}
		if target != skills.TargetCodex && target != skills.TargetClaude {
			return nil, fmt.Errorf("unknown skill harness %q (want codex or claude)", name)
		}
		result = append(result, target)
	}
	return normalizeSkillHarnesses(result)
}

func defaultSkillHarnesses() []skills.Target {
	return []skills.Target{skills.TargetCodex, skills.TargetClaude}
}

func newSkillRenderer(command *cobra.Command) *render.Renderer {
	writer := command.OutOrStdout()
	return render.NewRenderer(writer, render.AutoTheme(command.InOrStdin(), writer, os.Environ()))
}

func runSkillsService(command *cobra.Command, service SkillsService, request SkillsRequest) error {
	if request.Renderer == nil {
		request.Renderer = newSkillRenderer(command)
	}
	return service.Run(command.Context(), request)
}

func (s *skillsService) Run(ctx context.Context, request SkillsRequest) error {
	if s.err != nil {
		return s.err
	}
	if s.manager == nil {
		return fmt.Errorf("skills manager is unavailable")
	}
	if request.Writer == nil {
		request.Writer = io.Discard
	}
	if request.Renderer == nil {
		request.Renderer = render.NewPlainRenderer(request.Writer)
	}
	switch request.Action {
	case SkillsInstall:
		return s.runCatalogInstall(ctx, request)
	case SkillsList:
		infos, err := s.manager.List(skills.InspectOptions{Target: request.Target, Scope: request.Scope})
		if err != nil {
			return err
		}
		return renderSkillList(request.Writer, infos)
	case SkillsInfo:
		infos, err := s.manager.List(skills.InspectOptions{Target: request.Target, Scope: request.Scope, Names: request.Names})
		if err != nil {
			return err
		}
		if len(infos) == 0 {
			return fmt.Errorf("skill %q was not found", strings.Join(request.Names, ", "))
		}
		return renderSkillInfo(request.Writer, infos)
	case SkillsUpdate:
		return s.runCatalogUpdate(ctx, request)
	case SkillsRemove:
		results, err := s.manager.Remove(ctx, skills.RemoveOptions{Target: request.Target, Scope: request.Scope, Names: request.Names, DryRun: request.DryRun})
		if err != nil {
			return err
		}
		return renderSkillResults(request.Writer, results)
	case SkillsVerify:
		results, err := s.manager.Verify(ctx, skills.InspectOptions{Target: request.Target, Scope: request.Scope, Names: request.Names})
		if err != nil {
			return err
		}
		if err := renderSkillResults(request.Writer, results); err != nil {
			return err
		}
		for _, result := range results {
			if result.Status != "valid" {
				return fmt.Errorf("skill verification found %s skill %q", result.Status, result.Name)
			}
		}
		return nil
	case SkillsDoctor:
		results, err := s.manager.Doctor(ctx, skills.InspectOptions{Target: request.Target, Scope: request.Scope})
		if err != nil {
			return err
		}
		return renderSkillResults(request.Writer, results)
	default:
		return fmt.Errorf("unsupported skills action %q", request.Action)
	}
}

func (s *skillsService) runCatalogInstall(ctx context.Context, request SkillsRequest) error {
	entries, err := skills.ResolveCatalog(s.manager.Available(), request.Only)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(request.Writer, "No skills are available in the catalog.")
		return err
	}
	scope, err := chooseInstallScope(ctx, request)
	if err != nil {
		return err
	}
	harnesses, err := chooseInstallHarnesses(ctx, request)
	if err != nil {
		return err
	}
	entries = catalogEntriesForScopeAndHarnesses(entries, scope, harnesses)
	statuses, err := s.checkCatalog(ctx, request, entries, "Checking installed skills", false)
	if err != nil {
		return err
	}
	selected, err := chooseCatalogSkills(ctx, request, catalogInstallItems(statuses))
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		_, err := fmt.Fprintln(request.Writer, "No skills selected.")
		return err
	}
	results, err := s.manager.InstallCatalog(ctx, statuses, selected, skills.CatalogOperationOptions{
		DryRun:   request.DryRun,
		Progress: request.Renderer.Progress,
	})
	if err != nil {
		return err
	}
	return renderSkillResults(request.Writer, results)
}

func (s *skillsService) runCatalogUpdate(ctx context.Context, request SkillsRequest) error {
	entries, err := skills.ResolveCatalog(s.manager.Available(), request.Only)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(request.Writer, "No skills are available in the catalog.")
		return err
	}
	harnesses := defaultSkillHarnesses()
	if len(request.Harnesses) > 0 {
		harnesses, err = normalizeSkillHarnesses(request.Harnesses)
		if err != nil {
			return err
		}
	}
	entries = catalogEntriesForScopesAndHarnesses(entries, []skills.Scope{skills.ScopeUser, skills.ScopeProject}, harnesses)
	statuses, err := s.checkCatalog(ctx, request, entries, "Checking installed skills", true)
	if err != nil {
		return err
	}
	updateable := make([]skills.CatalogStatus, 0, len(statuses))
	for _, status := range statuses {
		if status.UpdateAvailable {
			updateable = append(updateable, status)
		}
	}
	if len(updateable) == 0 {
		_, err := fmt.Fprintln(request.Writer, "No skills can be updated.")
		return err
	}
	selected, err := chooseCatalogSkills(ctx, request, catalogUpdateItems(updateable))
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		_, err := fmt.Fprintln(request.Writer, "No skills selected.")
		return err
	}
	results, err := s.manager.UpdateCatalog(ctx, statuses, selected, skills.CatalogOperationOptions{
		DryRun:   request.DryRun,
		Progress: request.Renderer.Progress,
	})
	if err != nil {
		return err
	}
	return renderSkillResults(request.Writer, results)
}

func (s *skillsService) checkCatalog(ctx context.Context, request SkillsRequest, entries []skills.CatalogEntry, label string, checkUpdates bool) ([]skills.CatalogStatus, error) {
	if err := request.Renderer.ProgressBar(label, 0, len(entries)); err != nil {
		return nil, fmt.Errorf("render discovery progress: %w", err)
	}
	statuses, err := s.manager.CheckCatalog(ctx, entries, checkUpdates, func(completed, total int) error {
		return request.Renderer.ProgressBar(label, completed, total)
	})
	if err != nil {
		_ = request.Renderer.FinishProgress()
		return nil, err
	}
	return statuses, nil
}

func catalogEntriesForScopeAndHarnesses(entries []skills.CatalogEntry, scope skills.Scope, harnesses []skills.Target) []skills.CatalogEntry {
	result := make([]skills.CatalogEntry, 0, len(entries)*len(harnesses))
	for _, entry := range entries {
		for _, harness := range harnesses {
			entryCopy := entry
			entryCopy.Scope = scope
			entryCopy.Target = harness
			result = append(result, entryCopy)
		}
	}
	return result
}

func catalogEntriesForScopesAndHarnesses(entries []skills.CatalogEntry, scopes []skills.Scope, harnesses []skills.Target) []skills.CatalogEntry {
	result := make([]skills.CatalogEntry, 0, len(entries)*len(scopes)*len(harnesses))
	for _, scope := range scopes {
		result = append(result, catalogEntriesForScopeAndHarnesses(entries, scope, harnesses)...)
	}
	return result
}

func catalogInstallItems(statuses []skills.CatalogStatus) []render.Item {
	type groupedInstall struct {
		entry     skills.CatalogEntry
		installed []skills.Target
		partial   []skills.Target
		missing   []skills.Target
	}
	grouped := make([]groupedInstall, 0, len(statuses))
	indexes := make(map[skills.SkillID]int, len(statuses))
	for _, status := range statuses {
		index, exists := indexes[status.Entry.ID]
		if !exists {
			index = len(grouped)
			indexes[status.Entry.ID] = index
			grouped = append(grouped, groupedInstall{entry: status.Entry})
		}
		target := status.Entry.Target
		switch {
		case status.Installed:
			grouped[index].installed = append(grouped[index].installed, target)
		case status.PartiallyInstalled:
			grouped[index].partial = append(grouped[index].partial, target)
		default:
			grouped[index].missing = append(grouped[index].missing, target)
		}
	}
	items := make([]render.Item, 0, len(grouped))
	for _, install := range grouped {
		label := install.entry.Name
		if install.entry.Description != "" {
			label += " — " + install.entry.Description
		}
		if len(install.missing) > 0 {
			var details []string
			if len(install.installed) > 0 {
				details = append(details, "installed in "+joinSkillTargets(install.installed))
			}
			if len(install.partial) > 0 {
				details = append(details, "partially installed in "+joinSkillTargets(install.partial))
			}
			if len(install.missing) > 0 {
				details = append(details, "missing in "+joinSkillTargets(install.missing))
			}
			if len(details) > 0 {
				label += " (" + strings.Join(details, "; ") + ")"
			}
		} else if len(install.installed) > 0 {
			label += " (already installed in " + joinSkillTargets(install.installed) + ")"
		} else if len(install.partial) > 0 {
			label += " (partially installed in " + joinSkillTargets(install.partial) + ")"
		}
		items = append(items, render.Item{
			ID:       render.SelectionID(install.entry.ID),
			Name:     install.entry.Name,
			Group:    install.entry.Creator,
			Label:    label,
			Selected: len(install.missing) > 0,
			Disabled: len(install.missing) == 0,
		})
	}
	return items
}

func catalogUpdateItems(statuses []skills.CatalogStatus) []render.Item {
	type groupedUpdate struct {
		status    skills.CatalogStatus
		locations []string
	}
	grouped := make([]groupedUpdate, 0, len(statuses))
	indexes := make(map[skills.SkillID]int, len(statuses))
	for _, status := range statuses {
		if !status.UpdateAvailable {
			continue
		}
		index, exists := indexes[status.Entry.ID]
		if exists {
			grouped[index].locations = append(grouped[index].locations, displaySkillLocation(status.Entry))
			continue
		}
		indexes[status.Entry.ID] = len(grouped)
		grouped = append(grouped, groupedUpdate{status: status, locations: []string{displaySkillLocation(status.Entry)}})
	}
	items := make([]render.Item, 0, len(grouped))
	for _, update := range grouped {
		status := update.status
		label := status.Entry.Name
		if status.Entry.Description != "" {
			label += " — " + status.Entry.Description
		}
		if len(update.locations) > 0 {
			label += " (" + strings.Join(update.locations, ", ") + ")"
		}
		items = append(items, render.Item{
			ID:       render.SelectionID(status.Entry.ID),
			Name:     status.Entry.Name,
			Group:    status.Entry.Creator,
			Label:    label,
			Selected: true,
		})
	}
	return items
}

func displaySkillScope(scope skills.Scope) string {
	if scope == skills.ScopeProject {
		return "project"
	}
	return "global"
}

func chooseInstallScope(ctx context.Context, request SkillsRequest) (skills.Scope, error) {
	mode := request.ScopeMode
	if mode == "" {
		mode = skills.ScopeModeGlobal
	}
	parsed, err := skills.ParseScopeMode(string(mode))
	if err != nil {
		return "", err
	}
	if request.Yes || (request.ScopeSet && parsed != skills.ScopeModeChoose) || parsed == skills.ScopeModeProject {
		return skills.ScopeForMode(parsed), nil
	}
	if request.Input == nil {
		if request.DryRun {
			return skills.ScopeUser, nil
		}
		return "", fmt.Errorf("installation scope selection requires input")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintln(request.Writer, "Installation scope"); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintln(request.Writer, "1  Global (user-wide)"); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintln(request.Writer, "2  Project (current project)"); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(request.Writer, "Choose scope [1]: "); err != nil {
		return "", err
	}
	line, readErr := readSkillPromptLine(request.Input)
	if readErr != nil && (readErr != io.EOF || strings.TrimSpace(line) == "") {
		return "", fmt.Errorf("read installation scope: %w", readErr)
	}
	if _, err := fmt.Fprintln(request.Writer); err != nil {
		return "", err
	}
	return parseSelectedSkillScope(line)
}

func chooseInstallHarnesses(ctx context.Context, request SkillsRequest) ([]skills.Target, error) {
	if request.Yes {
		if len(request.Harnesses) > 0 {
			return normalizeSkillHarnesses(request.Harnesses)
		}
		return defaultSkillHarnesses(), nil
	}
	if request.HarnessesSet {
		return normalizeSkillHarnesses(request.Harnesses)
	}
	if request.HarnessSelection == nil {
		if request.DryRun {
			return defaultSkillHarnesses(), nil
		}
		return nil, fmt.Errorf("skill harness selection is required unless --yes is supplied")
	}
	items := []render.Item{
		{ID: render.SelectionID(skills.TargetCodex), Name: "Codex", Label: "Codex", Selected: true},
		{ID: render.SelectionID(skills.TargetClaude), Name: "Claude", Label: "Claude", Selected: true},
	}
	selected, err := request.HarnessSelection.Select(ctx, items)
	if errors.Is(err, render.ErrCancelled) {
		if renderErr := request.Renderer.Cancelled(); renderErr != nil {
			return nil, fmt.Errorf("render cancellation: %w", renderErr)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select skill harnesses: %w", err)
	}
	result := make([]skills.Target, 0, len(selected))
	for _, id := range selected {
		result = append(result, skills.Target(id))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one skill harness must be selected")
	}
	return normalizeSkillHarnesses(result)
}

func normalizeSkillHarnesses(harnesses []skills.Target) ([]skills.Target, error) {
	result := make([]skills.Target, 0, len(harnesses))
	seen := make(map[skills.Target]struct{}, len(harnesses))
	for _, target := range harnesses {
		if target != skills.TargetCodex && target != skills.TargetClaude {
			return nil, fmt.Errorf("unknown skill harness %q (want codex or claude)", target)
		}
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one skill harness is required")
	}
	return result, nil
}

func parseSelectedSkillScope(value string) (skills.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "global", "user":
		return skills.ScopeUser, nil
	case "2", "project":
		return skills.ScopeProject, nil
	default:
		return "", fmt.Errorf("unknown scope %q (want global or project)", strings.TrimSpace(value))
	}
}

func readSkillPromptLine(reader io.Reader) (string, error) {
	var line []byte
	buffer := []byte{0}
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			line = append(line, buffer[0])
			if buffer[0] == '\n' {
				return string(line), nil
			}
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				return string(line), io.EOF
			}
			return string(line), err
		}
	}
}

func joinSkillTargets(targets []skills.Target) string {
	values := make([]string, 0, len(targets))
	for _, target := range targets {
		if target == "" {
			continue
		}
		values = append(values, string(target))
	}
	if len(values) == 0 {
		return "selected destination"
	}
	return strings.Join(values, ", ")
}

func displaySkillLocation(entry skills.CatalogEntry) string {
	return displaySkillScope(entry.Scope) + "/" + string(entry.Target)
}

func chooseCatalogSkills(ctx context.Context, request SkillsRequest, items []render.Item) ([]skills.SkillID, error) {
	selected := selectionItemIDs(items)
	if !request.Yes && request.Selection == nil && !request.DryRun {
		return nil, fmt.Errorf("interactive skill selection is required unless --yes is supplied")
	}
	if !request.Yes && request.Selection != nil {
		ids, err := request.Selection.Select(ctx, items)
		if errors.Is(err, render.ErrCancelled) {
			if renderErr := request.Renderer.Cancelled(); renderErr != nil {
				return nil, fmt.Errorf("render cancellation: %w", renderErr)
			}
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("select skills: %w", err)
		}
		selected = ids
	}
	result := make([]skills.SkillID, 0, len(selected))
	for _, id := range selected {
		result = append(result, skills.SkillID(id))
	}
	return result, nil
}

func selectionItemIDs(items []render.Item) []render.SelectionID {
	ids := make([]render.SelectionID, 0, len(items))
	for _, item := range items {
		if item.Selected && !item.Disabled {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func renderSkillResults(writer io.Writer, results []skills.Result) error {
	if len(results) == 0 {
		_, err := fmt.Fprintln(writer, "No skills found.")
		return err
	}
	for _, result := range results {
		name := result.Name
		if name == "" {
			name = result.Path
		}
		scope := string(result.Target) + "/" + displaySkillScope(result.Scope)
		message := result.Message
		if message != "" {
			message = ": " + message
		}
		if _, err := fmt.Fprintf(writer, "• %-9s %-22s %s%s\n", result.Status, scope, name, message); err != nil {
			return err
		}
	}
	return nil
}

func renderSkillList(writer io.Writer, infos []skills.Info) error {
	if len(infos) == 0 {
		_, err := fmt.Fprintln(writer, "No skills found.")
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "TARGET\tSCOPE\tSKILL\tSTATUS\tDESCRIPTION"); err != nil {
		return err
	}
	for _, info := range infos {
		status := "installed"
		if !info.Valid {
			status = "invalid"
		} else if !info.Managed {
			status = "unmanaged"
		}
		description := info.Description
		if info.Error != "" {
			description = info.Error
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", info.Target, displaySkillScope(info.Scope), info.Name, status, description); err != nil {
			return err
		}
	}
	return table.Flush()
}

func renderSkillInfo(writer io.Writer, infos []skills.Info) error {
	for index, info := range infos {
		if index > 0 {
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(writer, "name: %s\ndescription: %s\ntarget: %s\nscope: %s\nstatus: %s\npath: %s\n", info.Name, info.Description, info.Target, displaySkillScope(info.Scope), skillInfoStatus(info), info.Path); err != nil {
			return err
		}
		if info.Source != "" {
			if _, err := fmt.Fprintf(writer, "source: %s\n", info.Source); err != nil {
				return err
			}
		}
		if info.Digest != "" {
			if _, err := fmt.Fprintf(writer, "digest: %s\n", info.Digest); err != nil {
				return err
			}
		}
	}
	return nil
}

func skillInfoStatus(info skills.Info) string {
	if !info.Valid {
		return "invalid"
	}
	if !info.Managed {
		return "unmanaged"
	}
	return "installed"
}

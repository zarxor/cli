// Package install selects and executes tool installation and update plans.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/zarxor/cli/internal/adapters"
	"github.com/zarxor/cli/internal/plan"
	"github.com/zarxor/cli/internal/platform"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/tools"
)

type SelectionUI = render.SelectionUI
type Item = render.Item

type Action string

const (
	Install Action = "install"
	Update  Action = "update"
)

type ToolStatus struct {
	Tool             tools.Tool
	Installed        bool
	Selected         bool
	CurrentVersion   string
	CandidateVersion string
}

type Options struct {
	Yes       bool
	DryRun    bool
	Writer    io.Writer
	Renderer  *render.Renderer
	Selection SelectionUI
}

type ToolResult struct {
	Tool   tools.Tool
	Action Action
	Status string
	Err    error
}

type Summary struct {
	Results   []ToolResult
	Failed    bool
	Cancelled bool
}

func Run(ctx context.Context, action Action, statuses []ToolStatus, adapterSet map[platform.OS]adapters.Adapter, opts Options) Summary {
	writer := opts.Writer
	if writer == nil {
		writer = io.Discard
	}
	renderer := opts.Renderer
	if renderer == nil {
		renderer = render.NewPlainRenderer(writer)
	}
	if action != Install && action != Update {
		return failedPlan(statuses, action, fmt.Errorf("unsupported action %q", action))
	}

	eligible := eligibleStatuses(action, statuses)
	if len(eligible) == 0 {
		if action == Update {
			if err := renderer.Progress("No tool updates available."); err != nil {
				return failedPlan(statuses, action, fmt.Errorf("render empty update plan: %w", err))
			}
			return Summary{}
		}
		if err := renderStatuses(renderer, eligible); err != nil {
			return failedPlan(statuses, action, fmt.Errorf("render plan: %w", err))
		}
		return Summary{}
	}
	items := selectionItems(action, eligible)
	selectedIDs := itemIDs(items)
	if !opts.Yes && !opts.DryRun && opts.Selection == nil {
		return failedPlan(eligible, action, fmt.Errorf("interactive selection is required unless --yes is supplied"))
	}
	if !opts.Yes && opts.Selection != nil {
		var err error
		selectedIDs, err = opts.Selection.Select(ctx, items)
		if errors.Is(err, render.ErrCancelled) {
			if renderErr := renderer.Cancelled(); renderErr != nil {
				return failedPlan(eligible, action, fmt.Errorf("render cancellation: %w", renderErr))
			}
			return Summary{Cancelled: true}
		}
		if err != nil {
			return failedPlan(eligible, action, fmt.Errorf("select tools: %w", err))
		}
	} else {
		if err := renderStatuses(renderer, eligible); err != nil {
			return failedPlan(eligible, action, fmt.Errorf("render plan: %w", err))
		}
	}

	selected := selectStatuses(action, eligible, selectedIDs)
	orderedTools, err := plan.DependencyOrder(statusTools(selected))
	if err != nil {
		return failedPlan(selected, action, err)
	}
	selectedByID := make(map[tools.ToolID]ToolStatus, len(selected))
	for _, status := range selected {
		selectedByID[status.Tool.ID] = status
	}

	summary := Summary{Results: make([]ToolResult, 0, len(orderedTools))}
	if len(orderedTools) == 0 {
		return summary
	}
	if opts.DryRun {
		for _, tool := range orderedTools {
			resultAction := actionFor(action, selectedByID[tool.ID])
			result := ToolResult{Tool: tool, Action: resultAction, Status: "dry-run"}
			summary.Results = append(summary.Results, result)
			if err := renderResult(renderer, result); err != nil {
				summary.Failed = true
				summary.Results[len(summary.Results)-1].Status = "failed"
				summary.Results[len(summary.Results)-1].Err = fmt.Errorf("render dry-run result: %w", err)
				return summary
			}
		}
		return summary
	}

	adapter, err := hostAdapter(adapterSet)
	if err != nil {
		return failedPlan(selected, action, err)
	}
	resultsByID := make(map[tools.ToolID]ToolResult, len(orderedTools))
	selectedSet := make(map[tools.ToolID]struct{}, len(orderedTools))
	for _, tool := range orderedTools {
		selectedSet[tool.ID] = struct{}{}
	}

	for _, tool := range orderedTools {
		status := selectedByID[tool.ID]
		resultAction := actionFor(action, status)
		if dependency, blocked := failedDependency(tool, selectedSet, resultsByID); blocked {
			result := ToolResult{
				Tool:   tool,
				Action: resultAction,
				Status: "skipped",
				Err:    fmt.Errorf("dependency %q did not complete successfully", dependency),
			}
			summary.Results = append(summary.Results, result)
			resultsByID[tool.ID] = result
			summary.Failed = true
			if err := renderResult(renderer, result); err != nil {
				summary.Results[len(summary.Results)-1].Status = "failed"
				summary.Results[len(summary.Results)-1].Err = fmt.Errorf("render skipped result: %w", err)
				return summary
			}
			continue
		}
		if status.Installed && versionsMatch(status) {
			result := ToolResult{Tool: tool, Action: resultAction, Status: "up-to-date"}
			summary.Results = append(summary.Results, result)
			resultsByID[tool.ID] = result
			if err := renderResult(renderer, result); err != nil {
				summary.Failed = true
				summary.Results[len(summary.Results)-1].Status = "failed"
				summary.Results[len(summary.Results)-1].Err = fmt.Errorf("render up-to-date result: %w", err)
				return summary
			}
			continue
		}

		verb := "Installing"
		if resultAction == Update {
			verb = "Updating"
		}
		if err := renderer.Progress(fmt.Sprintf("%s %s…", verb, tool.Name)); err != nil {
			result := ToolResult{
				Tool:   tool,
				Action: resultAction,
				Status: "failed",
				Err:    fmt.Errorf("render progress: %w", err),
			}
			summary.Results = append(summary.Results, result)
			resultsByID[tool.ID] = result
			summary.Failed = true
			return summary
		}
		err := execute(ctx, adapter, resultAction, tool)
		resultStatus := pastTense(resultAction)
		if err == nil {
			if progressErr := renderer.Progress(fmt.Sprintf("Verifying %s…", tool.Name)); progressErr != nil {
				err = fmt.Errorf("render progress: %w", progressErr)
			} else {
				err = adapter.Verify(ctx, tool)
			}
		}
		result := ToolResult{Tool: tool, Action: resultAction, Status: resultStatus, Err: err}
		if err != nil {
			result.Status = "failed"
			summary.Failed = true
		}
		summary.Results = append(summary.Results, result)
		resultsByID[tool.ID] = result
		if renderErr := renderResult(renderer, result); renderErr != nil {
			summary.Failed = true
			summary.Results[len(summary.Results)-1].Status = "failed"
			summary.Results[len(summary.Results)-1].Err = fmt.Errorf("render result: %w", renderErr)
			return summary
		}
	}
	return summary
}

func eligibleStatuses(action Action, statuses []ToolStatus) []ToolStatus {
	byID := make(map[tools.ToolID]int, len(statuses))
	eligible := make([]ToolStatus, 0, len(statuses))
	for _, status := range statuses {
		if action == Update && !status.Installed {
			continue
		}
		if index, exists := byID[status.Tool.ID]; exists {
			mergeStatus(&eligible[index], status)
			continue
		}
		byID[status.Tool.ID] = len(eligible)
		eligible = append(eligible, status)
	}
	if action == Update {
		updatable := eligible[:0]
		for _, status := range eligible {
			if hasUpdateCandidate(status) && !versionsMatch(status) {
				updatable = append(updatable, status)
			}
		}
		return updatable
	}
	return eligible
}

func mergeStatus(target *ToolStatus, duplicate ToolStatus) {
	target.Installed = target.Installed || duplicate.Installed
	target.Selected = target.Selected || duplicate.Selected
	if target.CurrentVersion == "" {
		target.CurrentVersion = duplicate.CurrentVersion
	}
	if target.CandidateVersion == "" {
		target.CandidateVersion = duplicate.CandidateVersion
	}
}

func selectionItems(action Action, statuses []ToolStatus) []Item {
	available := make([]Item, 0, len(statuses))
	installed := make([]Item, 0, len(statuses))
	for _, status := range statuses {
		label := status.Tool.Name
		disabled := action == Install && status.Installed
		if disabled {
			label = fmt.Sprintf("%s (already installed", status.Tool.Name)
			if strings.TrimSpace(status.CurrentVersion) != "" {
				label += ": " + status.CurrentVersion
			}
			label += ")"
		} else if action == Update {
			label = fmt.Sprintf("%s (%s -> %s)", status.Tool.Name, versionLabel(status.CurrentVersion), versionLabel(status.CandidateVersion))
		}
		item := Item{Tool: status.Tool, ID: render.SelectionID(status.Tool.ID), Name: status.Tool.Name, Label: label, Selected: !disabled, Disabled: disabled}
		if disabled {
			installed = append(installed, item)
		} else {
			available = append(available, item)
		}
	}
	return append(available, installed...)
}

func renderStatuses(renderer *render.Renderer, statuses []ToolStatus) error {
	rows := make([]render.VersionRow, 0, len(statuses))
	for _, status := range statuses {
		rows = append(rows, render.VersionRow{
			Tool:             status.Tool,
			CurrentVersion:   status.CurrentVersion,
			CandidateVersion: status.CandidateVersion,
		})
	}
	return renderer.VersionTable(rows)
}

func renderResult(renderer *render.Renderer, result ToolResult) error {
	return renderer.Result(render.ResultRow{
		Action: string(result.Action),
		Tool:   result.Tool.Name,
		Status: result.Status,
		Err:    result.Err,
	})
}

func itemIDs(items []Item) []render.SelectionID {
	ids := make([]render.SelectionID, 0, len(items))
	for _, item := range items {
		if item.Selected && !item.Disabled {
			id := item.ID
			if id == "" {
				id = render.SelectionID(item.Tool.ID)
			}
			ids = append(ids, id)
		}
	}
	return ids
}

func selectStatuses(action Action, statuses []ToolStatus, ids []render.SelectionID) []ToolStatus {
	selected := make(map[tools.ToolID]struct{}, len(ids))
	for _, id := range ids {
		selected[tools.ToolID(id)] = struct{}{}
	}
	result := make([]ToolStatus, 0, len(selected))
	for _, status := range statuses {
		if action == Install && status.Installed {
			continue
		}
		if _, ok := selected[status.Tool.ID]; ok {
			result = append(result, status)
		}
	}
	return result
}

func statusTools(statuses []ToolStatus) []tools.Tool {
	result := make([]tools.Tool, len(statuses))
	for i, status := range statuses {
		result[i] = status.Tool
	}
	return result
}

func actionFor(requested Action, status ToolStatus) Action {
	if requested == Install && status.Installed {
		return Update
	}
	return requested
}

func versionsMatch(status ToolStatus) bool {
	current := canonicalVersion(status.CurrentVersion)
	candidate := canonicalVersion(status.CandidateVersion)
	return current != "" && candidate != "" && current == candidate
}

func hasUpdateCandidate(status ToolStatus) bool {
	return strings.TrimSpace(status.CandidateVersion) != ""
}

var numericVersion = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)+`)

func canonicalVersion(version string) string {
	// Git for Windows embeds its platform marker between the patch and build
	// components (for example, 2.55.0.windows.3), while package metadata writes
	// the same version as 2.55.0.3.
	normalized := strings.ReplaceAll(strings.ToLower(version), ".windows.", ".")
	if numeric := numericVersion.FindString(normalized); numeric != "" {
		return numeric
	}
	return strings.TrimSpace(normalized)
}

func execute(ctx context.Context, adapter adapters.Adapter, action Action, tool tools.Tool) error {
	if action == Install {
		return adapter.Install(ctx, tool)
	}
	return adapter.Update(ctx, tool)
}

func failedDependency(tool tools.Tool, selected map[tools.ToolID]struct{}, results map[tools.ToolID]ToolResult) (tools.ToolID, bool) {
	for _, dependency := range tool.Dependencies {
		if _, isSelected := selected[dependency]; !isSelected {
			continue
		}
		result := results[dependency]
		if result.Err != nil || result.Status == "skipped" {
			return dependency, true
		}
	}
	return "", false
}

func hostAdapter(adapterSet map[platform.OS]adapters.Adapter) (adapters.Adapter, error) {
	host, err := platform.Detect()
	if err == nil {
		if adapter := adapterSet[host]; adapter != nil {
			return adapter, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("detect platform: %w", err)
	}
	return nil, fmt.Errorf("no adapter configured for platform %q", host)
}

func failedPlan(statuses []ToolStatus, action Action, err error) Summary {
	summary := Summary{Failed: true, Results: make([]ToolResult, 0, len(statuses))}
	for _, status := range statuses {
		summary.Results = append(summary.Results, ToolResult{Tool: status.Tool, Action: action, Status: "failed", Err: err})
	}
	return summary
}

func pastTense(action Action) string {
	if action == Install {
		return "installed"
	}
	return "updated"
}

func versionLabel(version string) string {
	if strings.TrimSpace(version) == "" {
		return "-"
	}
	return version
}

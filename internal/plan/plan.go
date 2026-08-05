// Package plan turns profile and tool selections into an install plan.
package plan

import (
	"fmt"

	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/tools"
)

func MergeProfiles(profiles []profile.Profile, only []profile.ToolID) ([]profile.Tool, error) {
	selected := make(map[profile.ToolID]struct{})

	if len(profiles) == 0 {
		for _, id := range only {
			if _, ok := tools.Lookup(id); !ok {
				return nil, fmt.Errorf("unknown tool %q", id)
			}
			selected[id] = struct{}{}
		}
	} else {
		allowed := make(map[profile.ToolID]struct{})
		for _, selectedProfile := range profiles {
			for _, id := range selectedProfile.ToolIDs {
				if _, ok := tools.Lookup(id); !ok {
					return nil, fmt.Errorf("unknown tool %q", id)
				}
				allowed[id] = struct{}{}
			}
		}

		if len(only) == 0 {
			selected = allowed
		} else {
			for _, id := range only {
				if _, ok := tools.Lookup(id); !ok {
					return nil, fmt.Errorf("unknown tool %q", id)
				}
				if _, ok := allowed[id]; ok {
					selected[id] = struct{}{}
				}
			}
		}
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("tool selection is empty")
	}

	expanded := make(map[profile.ToolID]struct{})
	for id := range selected {
		if err := addDependencies(id, expanded); err != nil {
			return nil, err
		}
	}

	planned := make([]profile.Tool, 0, len(expanded))
	for _, tool := range tools.Catalog {
		if _, ok := expanded[tool.ID]; ok {
			planned = append(planned, tool)
		}
	}
	return planned, nil
}

func addDependencies(id profile.ToolID, selected map[profile.ToolID]struct{}) error {
	if _, ok := selected[id]; ok {
		return nil
	}
	tool, ok := tools.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	selected[id] = struct{}{}
	for _, dependency := range tool.Dependencies {
		if err := addDependencies(dependency, selected); err != nil {
			return err
		}
	}
	return nil
}

// DependencyOrder returns each selected tool once with selected dependencies
// before their dependents. Dependencies absent from the selection are left
// alone, which preserves an explicit interactive deselection.
func DependencyOrder(selected []tools.Tool) ([]tools.Tool, error) {
	unique := make([]tools.Tool, 0, len(selected))
	byID := make(map[tools.ToolID]tools.Tool, len(selected))
	for _, tool := range selected {
		if _, exists := byID[tool.ID]; exists {
			continue
		}
		byID[tool.ID] = tool
		unique = append(unique, tool)
	}

	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[tools.ToolID]int, len(unique))
	ordered := make([]tools.Tool, 0, len(unique))
	var visit func(tools.Tool) error
	visit = func(tool tools.Tool) error {
		switch state[tool.ID] {
		case visiting:
			return fmt.Errorf("dependency cycle includes tool %q", tool.ID)
		case visited:
			return nil
		}
		state[tool.ID] = visiting
		for _, dependencyID := range tool.Dependencies {
			if dependency, selected := byID[dependencyID]; selected {
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		state[tool.ID] = visited
		ordered = append(ordered, tool)
		return nil
	}

	for _, tool := range unique {
		if err := visit(tool); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

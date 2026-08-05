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

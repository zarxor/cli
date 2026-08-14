// Package profile defines the named groups of tools supported by jb.
package profile

import "fmt"

type ToolID string
type ProfileName string

type Tool struct {
	ID           ToolID
	Name         string
	Includes     []ToolID
	Dependencies []ToolID
}

type Profile struct {
	Name     ProfileName
	ToolIDs  []ToolID
	Includes []ProfileName
}

const (
	Git           ToolID      = "git"
	GitHubCLI     ToolID      = "github-cli"
	Docker        ToolID      = "docker"
	DockerBuildx  ToolID      = "docker-buildx"
	DockerCompose ToolID      = "docker-compose"
	Claude        ToolID      = "claude"
	Codex         ToolID      = "codex"
	T3Code        ToolID      = "t3-code"
	Mise          ToolID      = "mise"
	UV            ToolID      = "uv"
	OpenCode      ToolID      = "opencode"
	NVM           ToolID      = "nvm"
	Node          ToolID      = "node"
	NPM           ToolID      = "npm"
	Corepack      ToolID      = "corepack"
	PNPM          ToolID      = "pnpm"
	Yarn          ToolID      = "yarn"
	Bun           ToolID      = "bun"
	Development   ProfileName = "development"
	Desktop       ProfileName = "desktop"
	Server        ProfileName = "server"
	Core          ProfileName = "core"
	Agents        ProfileName = "agents"
	Containers    ProfileName = "containers"
	JavaScript    ProfileName = "javascript"
	Python        ProfileName = "python"
	Optional      ProfileName = "optional"
)

func DevelopmentProfile() Profile {
	return resolveBuiltInProfile(Development)
}

// DesktopProfile is the full local development profile, including the
// installed agent CLIs and T3 Code.
func DesktopProfile() Profile {
	return resolveBuiltInProfile(Desktop)
}

// ServerProfile contains the tools useful for a headless host while leaving
// T3 Code and alternate-runtime tools out of the automatic plan.
func ServerProfile() Profile {
	return resolveBuiltInProfile(Server)
}

// BuiltInProfiles returns focused profiles and the larger profiles composed
// from them. Resolved profiles returned by ResolveProfiles contain only their
// final tool IDs, keeping callers independent from the composition details.
func BuiltInProfiles() []Profile {
	return []Profile{
		{Name: Core, ToolIDs: []ToolID{Git, GitHubCLI}},
		{Name: Agents, ToolIDs: []ToolID{Claude, Codex, T3Code}},
		{Name: Containers, ToolIDs: []ToolID{Docker}},
		{Name: JavaScript, ToolIDs: []ToolID{Node, T3Code, Bun}},
		{Name: Python, ToolIDs: []ToolID{UV}},
		{Name: Optional, ToolIDs: []ToolID{Mise, UV, OpenCode}},
		{Name: Development, Includes: []ProfileName{Core, Containers}, ToolIDs: []ToolID{Claude, Codex, Node, T3Code, Bun}},
		{Name: Desktop, Includes: []ProfileName{Development}},
		{Name: Server, Includes: []ProfileName{Core, Containers}, ToolIDs: []ToolID{Claude, Codex, Node}},
	}
}

func ResolveProfiles(names []string) ([]Profile, error) {
	definitions := make(map[ProfileName]Profile)
	for _, selected := range BuiltInProfiles() {
		definitions[selected.Name] = selected
	}
	profiles := make([]Profile, 0, len(names))
	for _, name := range names {
		selected, err := resolveProfile(ProfileName(name), definitions, make(map[ProfileName]bool))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, selected)
	}
	return profiles, nil
}

func resolveBuiltInProfile(name ProfileName) Profile {
	profiles, err := ResolveProfiles([]string{string(name)})
	if err != nil {
		return Profile{Name: name}
	}
	return profiles[0]
}

func resolveProfile(name ProfileName, definitions map[ProfileName]Profile, visiting map[ProfileName]bool) (Profile, error) {
	selected, ok := definitions[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown profile %q", name)
	}
	if visiting[name] {
		return Profile{}, fmt.Errorf("profile include cycle includes %q", name)
	}
	visiting[name] = true
	defer delete(visiting, name)

	result := Profile{Name: name}
	seen := make(map[ToolID]struct{})
	appendTools := func(ids []ToolID) {
		for _, id := range ids {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result.ToolIDs = append(result.ToolIDs, id)
		}
	}
	for _, included := range selected.Includes {
		includedProfile, err := resolveProfile(included, definitions, visiting)
		if err != nil {
			return Profile{}, err
		}
		appendTools(includedProfile.ToolIDs)
	}
	appendTools(selected.ToolIDs)
	return result, nil
}

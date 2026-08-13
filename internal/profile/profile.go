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
	Name    ProfileName
	ToolIDs []ToolID
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
)

func DevelopmentProfile() Profile {
	return Profile{
		Name:    Development,
		ToolIDs: developmentToolIDs(),
	}
}

// DesktopProfile is the full local development profile, including the
// installed agent CLIs and T3 Code.
func DesktopProfile() Profile {
	return Profile{
		Name:    Desktop,
		ToolIDs: developmentToolIDs(),
	}
}

// ServerProfile contains the tools useful for a headless host while leaving
// T3 Code and alternate-runtime tools out of the automatic plan.
func ServerProfile() Profile {
	return Profile{
		Name: Server,
		ToolIDs: []ToolID{
			Git, GitHubCLI, Docker, Claude, Codex, Node,
		},
	}
}

func developmentToolIDs() []ToolID {
	return []ToolID{Git, GitHubCLI, Docker, Claude, Codex, Node, T3Code, Bun}
}

func ResolveProfiles(names []string) ([]Profile, error) {
	profiles := make([]Profile, 0, len(names))
	for _, name := range names {
		switch ProfileName(name) {
		case Development:
			profiles = append(profiles, DevelopmentProfile())
		case Desktop:
			profiles = append(profiles, DesktopProfile())
		case Server:
			profiles = append(profiles, ServerProfile())
		default:
			return nil, fmt.Errorf("unknown profile %q", name)
		}
	}
	return profiles, nil
}

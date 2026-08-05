// Package profile defines the named groups of tools supported by jb.
package profile

import "fmt"

type ToolID string
type ProfileName string

type Tool struct {
	ID           ToolID
	Name         string
	Dependencies []ToolID
}

type Profile struct {
	Name    ProfileName
	ToolIDs []ToolID
}

const (
	Git           ToolID = "git"
	GitHubCLI     ToolID = "github-cli"
	Docker        ToolID = "docker"
	DockerBuildx  ToolID = "docker-buildx"
	DockerCompose ToolID = "docker-compose"
	Codex         ToolID = "codex"
	NVM           ToolID = "nvm"
	Node          ToolID = "node"
	NPM           ToolID = "npm"
	Corepack      ToolID = "corepack"
	PNPM          ToolID = "pnpm"
	Yarn          ToolID = "yarn"
	Bun           ToolID = "bun"

	Development ProfileName = "development"
)

func DevelopmentProfile() Profile {
	return Profile{
		Name: Development,
		ToolIDs: []ToolID{
			Git, GitHubCLI, Docker, Codex, Node, Bun,
		},
	}
}

func ResolveProfiles(names []string) ([]Profile, error) {
	profiles := make([]Profile, 0, len(names))
	for _, name := range names {
		switch ProfileName(name) {
		case Development:
			profiles = append(profiles, DevelopmentProfile())
		default:
			return nil, fmt.Errorf("unknown profile %q", name)
		}
	}
	return profiles, nil
}

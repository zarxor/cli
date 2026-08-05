// Package tools supplies the stable catalog used for all jb plans.
package tools

import (
	"fmt"

	"github.com/zarxor/scripts/internal/profile"
)

var Catalog = []profile.Tool{
	{ID: profile.Git, Name: "Git"},
	{ID: profile.GitHubCLI, Name: "GitHub CLI"},
	{ID: profile.Docker, Name: "Docker", Dependencies: []profile.ToolID{profile.DockerBuildx, profile.DockerCompose}},
	{ID: profile.DockerBuildx, Name: "Docker Buildx"},
	{ID: profile.DockerCompose, Name: "Docker Compose"},
	{ID: profile.Codex, Name: "Codex"},
	{ID: profile.NVM, Name: "nvm"},
	{ID: profile.Node, Name: "Node.js LTS", Dependencies: []profile.ToolID{profile.NVM, profile.NPM, profile.Corepack, profile.PNPM, profile.Yarn}},
	{ID: profile.NPM, Name: "npm"},
	{ID: profile.Corepack, Name: "Corepack"},
	{ID: profile.PNPM, Name: "pnpm"},
	{ID: profile.Yarn, Name: "Yarn"},
	{ID: profile.Bun, Name: "Bun"},
}

var byID = buildLookup(Catalog)

func ResolveTools(ids []profile.ToolID) ([]profile.Tool, error) {
	resolved := make([]profile.Tool, 0, len(ids))
	for _, id := range ids {
		tool, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q", id)
		}
		resolved = append(resolved, tool)
	}
	return resolved, nil
}

func Lookup(id profile.ToolID) (profile.Tool, bool) {
	tool, ok := byID[id]
	return tool, ok
}

func buildLookup(catalog []profile.Tool) map[profile.ToolID]profile.Tool {
	lookup := make(map[profile.ToolID]profile.Tool, len(catalog))
	for _, tool := range catalog {
		lookup[tool.ID] = tool
	}
	return lookup
}

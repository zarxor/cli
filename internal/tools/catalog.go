// Package tools supplies the stable catalog used for all jb plans.
package tools

import (
	"fmt"

	"github.com/zarxor/scripts/internal/profile"
)

// Tool and ToolID are the catalog boundary types consumed by adapters and
// planners. They remain aliases so profile definitions have one source of
// truth while callers do not need to depend on the profile package.
type Tool = profile.Tool
type ToolID = profile.ToolID

var Catalog = []profile.Tool{
	{ID: profile.Git, Name: "Git"},
	{ID: profile.GitHubCLI, Name: "GitHub CLI"},
	{ID: profile.Docker, Name: "Docker", Includes: []profile.ToolID{profile.DockerBuildx, profile.DockerCompose}},
	{ID: profile.DockerBuildx, Name: "Docker Buildx", Dependencies: []profile.ToolID{profile.Docker}},
	{ID: profile.DockerCompose, Name: "Docker Compose", Dependencies: []profile.ToolID{profile.Docker}},
	{ID: profile.NVM, Name: "nvm", Dependencies: []profile.ToolID{profile.Git}},
	{ID: profile.Node, Name: "Node.js LTS", Includes: []profile.ToolID{profile.NPM, profile.Corepack, profile.PNPM, profile.Yarn}, Dependencies: []profile.ToolID{profile.NVM}},
	{ID: profile.NPM, Name: "npm", Dependencies: []profile.ToolID{profile.Node}},
	{ID: profile.Corepack, Name: "Corepack", Dependencies: []profile.ToolID{profile.NPM}},
	{ID: profile.PNPM, Name: "pnpm", Dependencies: []profile.ToolID{profile.Corepack}},
	{ID: profile.Yarn, Name: "Yarn", Dependencies: []profile.ToolID{profile.Corepack}},
	{ID: profile.Codex, Name: "Codex", Dependencies: []profile.ToolID{profile.NPM}},
	{ID: profile.Bun, Name: "Bun", Dependencies: []profile.ToolID{profile.NPM}},
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

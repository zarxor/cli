package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/runner"
	"github.com/zarxor/cli/internal/tools"
)

// DarwinAdapter installs macOS tools through Homebrew. Runtime CLIs that are
// published to npm use the Homebrew Node runtime and remain user-level npm
// operations; no shell profile evaluation is required during a run.
type DarwinAdapter struct {
	runner    runner.Runner
	elevation runner.Elevation
}

type DarwinConfig struct {
	Home string
}

type brewSource struct {
	formula    string
	executable string
	version    []string
	cask       bool
	listing    bool
}

var darwinSources = map[tools.ToolID]brewSource{
	profile.Git:           {formula: "git", executable: "git", version: []string{"--version"}},
	profile.GitHubCLI:     {formula: "gh", executable: "gh", version: []string{"--version"}},
	profile.Docker:        {formula: "docker", executable: "docker", version: []string{"--version"}, cask: true},
	profile.DockerBuildx:  {executable: "docker", version: []string{"buildx", "version"}},
	profile.DockerCompose: {executable: "docker", version: []string{"compose", "version"}},
	profile.NVM:           {formula: "nvm", executable: "brew", version: []string{"list", "--versions", "nvm"}, listing: true},
	profile.Node:          {formula: "node", executable: "node", version: []string{"--version"}},
	profile.Bun:           {formula: "bun", executable: "bun", version: []string{"--version"}},
	profile.Mise:          {formula: "mise", executable: "mise", version: []string{"--version"}},
	profile.UV:            {formula: "uv", executable: "uv", version: []string{"--version"}},
	profile.UVX:           {formula: "uv", executable: "uvx", version: []string{"--version"}},
}

var darwinNPMCommands = map[tools.ToolID]struct {
	executable  string
	args        []string
	packageName string
}{
	profile.NPM:      {executable: "npm", args: []string{"--version"}, packageName: "npm"},
	profile.Corepack: {executable: "corepack", args: []string{"--version"}, packageName: "corepack"},
	profile.PNPM:     {executable: "pnpm", args: []string{"--version"}, packageName: "pnpm"},
	profile.Yarn:     {executable: "yarn", args: []string{"--version"}, packageName: "@yarnpkg/cli-dist"},
	profile.Claude:   {executable: "claude", args: []string{"--version"}, packageName: "@anthropic-ai/claude-code"},
	profile.Codex:    {executable: "codex", args: []string{"--version"}, packageName: "@openai/codex"},
	profile.T3Code:   {executable: "t3", args: []string{"--version"}, packageName: "t3"},
	profile.OpenCode: {executable: "opencode", args: []string{"--version"}, packageName: "opencode-ai"},
}

func NewDarwinAdapter(commandRunner runner.Runner, elevation runner.Elevation, configs ...DarwinConfig) Adapter {
	if elevation == nil {
		elevation = darwinElevation{runner: commandRunner}
	}
	return &DarwinAdapter{runner: commandRunner, elevation: elevation}
}

func (a *DarwinAdapter) Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	if source, ok := darwinSources[tool.ID]; ok {
		return a.detectBrewSource(ctx, tool, source)
	}
	command, ok := darwinNPMCommands[tool.ID]
	if !ok {
		return detect.Detection{}, a.unsupported(tool)
	}
	if _, err := a.runner.LookPath(ctx, command.executable); err != nil {
		return detect.Detection{}, nil
	}
	result, err := a.runner.Run(ctx, command.executable, command.args...)
	if err != nil {
		return detect.Detection{}, err
	}
	detection := detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}
	if candidate, candidateErr := a.npmCandidate(ctx, command.packageName); candidateErr == nil {
		detection.Candidate = candidate
	}
	return detection, nil
}

func (a *DarwinAdapter) Install(ctx context.Context, tool tools.Tool) error {
	if source, ok := darwinSources[tool.ID]; ok {
		if isDarwinDockerComponent(tool.ID) {
			return nil
		}
		if tool.ID == profile.UVX {
			// Homebrew's uv formula supplies both uv and uvx.
			return nil
		}
		if err := a.ensureBrew(ctx); err != nil {
			return err
		}
		args := []string{"install"}
		if source.cask {
			args = append(args, "--cask")
		}
		if err := a.brewRun(ctx, args[0], append(args[1:], source.formula)...); err != nil {
			return err
		}
		return nil
	}
	return a.installNPMTool(ctx, tool)
}

func (a *DarwinAdapter) Update(ctx context.Context, tool tools.Tool) error {
	if source, ok := darwinSources[tool.ID]; ok {
		if isDarwinDockerComponent(tool.ID) {
			return nil
		}
		if tool.ID == profile.UVX {
			// Homebrew's uv formula supplies both uv and uvx.
			return nil
		}
		if err := a.ensureBrew(ctx); err != nil {
			return err
		}
		args := []string{"upgrade"}
		if source.cask {
			args = append(args, "--cask")
		}
		if err := a.brewRun(ctx, args[0], append(args[1:], source.formula)...); err != nil {
			return err
		}
		return nil
	}
	return a.installNPMTool(ctx, tool)
}

func (a *DarwinAdapter) Verify(ctx context.Context, tool tools.Tool) error {
	detection, err := a.Detect(ctx, tool)
	if err != nil {
		return err
	}
	if !detection.Installed || detection.Current == "" {
		return fmt.Errorf("%s is not available after installation", tool.Name)
	}
	return nil
}

func (a *DarwinAdapter) detectBrewSource(ctx context.Context, tool tools.Tool, source brewSource) (detect.Detection, error) {
	if source.listing {
		if _, err := a.ensureBrewPath(ctx); err != nil {
			return detect.Detection{}, nil
		}
		result, err := a.runner.Run(ctx, source.executable, source.version...)
		if err != nil || strings.TrimSpace(result.Stdout+result.Stderr) == "" {
			return detect.Detection{}, nil
		}
		return detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}, nil
	}
	if _, err := a.runner.LookPath(ctx, source.executable); err != nil {
		return detect.Detection{}, nil
	}
	result, err := a.runner.Run(ctx, source.executable, source.version...)
	if err != nil {
		if isDarwinDockerComponent(tool.ID) {
			return detect.Detection{}, nil
		}
		return detect.Detection{}, err
	}
	detection := detect.Detection{Installed: true, Current: detect.ParseVersion(result.Stdout, result.Stderr)}
	if source.formula != "" {
		if candidate, candidateErr := a.brewCandidate(ctx, source); candidateErr == nil {
			detection.Candidate = candidate
		}
	}
	return detection, nil
}

func (a *DarwinAdapter) installNPMTool(ctx context.Context, tool tools.Tool) error {
	command, ok := darwinNPMCommands[tool.ID]
	if !ok {
		return a.unsupported(tool)
	}
	switch tool.ID {
	case profile.Corepack:
		if err := a.run(ctx, "npm", "install", "--global", "corepack@latest"); err != nil {
			return err
		}
		return a.run(ctx, "corepack", "enable")
	case profile.PNPM:
		return a.run(ctx, "corepack", "prepare", "pnpm@latest", "--activate")
	case profile.Yarn:
		return a.run(ctx, "corepack", "prepare", "yarn@stable", "--activate")
	default:
		return a.run(ctx, "npm", "install", "--global", command.packageName+"@latest")
	}
}

func (a *DarwinAdapter) npmCandidate(ctx context.Context, packageName string) (string, error) {
	result, err := a.runner.Run(ctx, "npm", "view", packageName, "version")
	if err != nil {
		return "", err
	}
	return detect.ParseVersion(result.Stdout, result.Stderr), nil
}

func (a *DarwinAdapter) brewCandidate(ctx context.Context, source brewSource) (string, error) {
	args := []string{"info", "--json=v2"}
	if source.cask {
		args = append(args, "--cask")
	}
	args = append(args, source.formula)
	result, err := a.runner.Run(ctx, "brew", args...)
	if err != nil {
		return "", err
	}
	var payload struct {
		Formulae []struct {
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
		} `json:"formulae"`
		Casks []struct {
			Version string `json:"version"`
		} `json:"casks"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		return "", err
	}
	if source.cask && len(payload.Casks) > 0 {
		return payload.Casks[0].Version, nil
	}
	if len(payload.Formulae) > 0 {
		return payload.Formulae[0].Versions.Stable, nil
	}
	return "", fmt.Errorf("brew returned no metadata for %s", source.formula)
}

func (a *DarwinAdapter) ensureBrew(ctx context.Context) error {
	if _, err := a.ensureBrewPath(ctx); err != nil {
		return fmt.Errorf("Homebrew is required on macOS; install it from https://brew.sh")
	}
	return nil
}

func (a *DarwinAdapter) ensureBrewPath(ctx context.Context) (string, error) {
	return a.runner.LookPath(ctx, "brew")
}

func (a *DarwinAdapter) run(ctx context.Context, command string, args ...string) error {
	_, err := a.runner.Run(ctx, command, args...)
	return err
}

func (a *DarwinAdapter) brewRun(ctx context.Context, command string, args ...string) error {
	_, err := a.runner.Run(ctx, "brew", append([]string{command}, args...)...)
	return err
}

func isDarwinDockerComponent(id tools.ToolID) bool {
	return id == profile.DockerBuildx || id == profile.DockerCompose
}

func (a *DarwinAdapter) unsupported(tool tools.Tool) error {
	return fmt.Errorf("macOS adapter does not support an installation path for %q", tool.ID)
}

type darwinElevation struct{ runner runner.Runner }

func (e darwinElevation) RunElevated(ctx context.Context, command string, args ...string) (runner.Result, error) {
	return e.runner.Run(ctx, "sudo", append([]string{command}, args...)...)
}

var _ Adapter = (*DarwinAdapter)(nil)

package adapters

import (
	"context"

	"github.com/zarxor/scripts/internal/detect"
	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/runner"
	"github.com/zarxor/scripts/internal/tools"
)

// ArchAdapter installs distribution packages through pacman and reuses the
// official user-level installers shared by Linux platforms.
type ArchAdapter struct{ linuxAdapter }

func NewArchAdapter(commandRunner runner.Runner, elevation runner.Elevation, configs ...LinuxConfig) Adapter {
	return &ArchAdapter{linuxAdapter: newLinuxAdapter(commandRunner, elevation, configs...)}
}

func (a *ArchAdapter) Detect(ctx context.Context, tool tools.Tool) (detect.Detection, error) {
	detection, err := a.detect(ctx, tool)
	if err != nil {
		return detection, err
	}
	packageName, ok := archPackage(tool.ID)
	if !ok {
		return detection, nil
	}
	result, err := a.runner.Run(ctx, "pacman", "-Si", packageName)
	if err != nil {
		return detection, nil
	}
	detection.Candidate = labeledValue(result.Stdout, "Version")
	return detection, nil
}

func (a *ArchAdapter) Install(ctx context.Context, tool tools.Tool) error {
	if packageName, ok := archPackage(tool.ID); ok {
		if err := a.system(ctx, "pacman", "-Syu", "--noconfirm", "--needed", packageName); err != nil {
			return err
		}
		if tool.ID == profile.Docker {
			return a.system(ctx, "systemctl", "enable", "--now", "docker.service")
		}
		return nil
	}
	if tool.ID == profile.Bun {
		if err := a.system(ctx, "pacman", "-Syu", "--noconfirm", "--needed", "unzip"); err != nil {
			return err
		}
	}
	return a.installUserTool(ctx, tool)
}

func (a *ArchAdapter) Update(ctx context.Context, tool tools.Tool) error {
	if packageName, ok := archPackage(tool.ID); ok {
		return a.system(ctx, "pacman", "-Syu", "--noconfirm", "--needed", packageName)
	}
	return a.updateUserTool(ctx, tool)
}

func (a *ArchAdapter) Verify(ctx context.Context, tool tools.Tool) error {
	return a.verify(ctx, tool)
}

func archPackage(id tools.ToolID) (string, bool) {
	switch id {
	case profile.Git:
		return "git", true
	case profile.GitHubCLI:
		return "github-cli", true
	case profile.Docker:
		return "docker", true
	case profile.DockerBuildx:
		return "docker-buildx", true
	case profile.DockerCompose:
		return "docker-compose", true
	default:
		return "", false
	}
}

var _ Adapter = (*DebianAdapter)(nil)
var _ Adapter = (*ArchAdapter)(nil)

package release

import (
	"context"
	"fmt"
	"strings"
)

type environment struct {
	root         string
	head         string
	git          string
	goExe        string
	gh           string
	shell        string
	tags         []string
	existingTags map[string]struct{}
}

func preflight(ctx context.Context, runner Runner, launchDir, hostOS string) (environment, error) {
	git, err := requireCommand(runner, "git")
	if err != nil {
		return environment{}, err
	}
	goExe, err := requireCommand(runner, "go")
	if err != nil {
		return environment{}, err
	}
	gh, err := requireCommand(runner, "gh")
	if err != nil {
		return environment{}, err
	}
	shellName := "bash"
	if hostOS == "windows" {
		shellName = "pwsh"
	}
	shell, err := requireCommand(runner, shellName)
	if err != nil {
		return environment{}, err
	}

	rootOutput, err := runner.Run(ctx, launchDir, git, "rev-parse", "--show-toplevel")
	if err != nil {
		return environment{}, fmt.Errorf("locate repository root: %w", err)
	}
	root := strings.TrimSpace(rootOutput)
	if root == "" {
		return environment{}, fmt.Errorf("locate repository root: git returned an empty path")
	}
	if _, err := runner.Run(ctx, root, gh, "auth", "status"); err != nil {
		return environment{}, fmt.Errorf("GitHub authentication failed; run gh auth login: %w", err)
	}
	branchOutput, err := runner.Run(ctx, root, git, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return environment{}, fmt.Errorf("read current branch: %w", err)
	}
	branch := strings.TrimSpace(branchOutput)
	if branch != "main" {
		return environment{}, fmt.Errorf("release requires branch main; current branch is %q", branch)
	}
	status, err := runner.Run(ctx, root, git, "status", "--porcelain")
	if err != nil {
		return environment{}, fmt.Errorf("inspect working tree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return environment{}, fmt.Errorf("working tree is not clean")
	}
	origin, err := runner.Run(ctx, root, git, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(origin) == "" {
		if err == nil {
			err = fmt.Errorf("empty URL")
		}
		return environment{}, fmt.Errorf("remote origin is unavailable: %w", err)
	}
	if _, err := runner.Run(ctx, root, git, "fetch", "origin", "main", "--tags"); err != nil {
		return environment{}, fmt.Errorf("fetch origin/main and tags: %w", err)
	}
	headOutput, err := runner.Run(ctx, root, git, "rev-parse", "HEAD")
	if err != nil {
		return environment{}, fmt.Errorf("read local HEAD: %w", err)
	}
	remoteOutput, err := runner.Run(ctx, root, git, "rev-parse", "origin/main")
	if err != nil {
		return environment{}, fmt.Errorf("read origin/main: %w", err)
	}
	head := strings.TrimSpace(headOutput)
	remoteHead := strings.TrimSpace(remoteOutput)
	if head == "" || head != remoteHead {
		return environment{}, fmt.Errorf("local HEAD %q does not match origin/main %q", head, remoteHead)
	}
	tagOutput, err := runner.Run(ctx, root, git, "tag", "--list")
	if err != nil {
		return environment{}, fmt.Errorf("list Git tags: %w", err)
	}
	tags := nonemptyLines(tagOutput)
	existingTags := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		existingTags[tag] = struct{}{}
	}

	return environment{
		root:         root,
		head:         head,
		git:          git,
		goExe:        goExe,
		gh:           gh,
		shell:        shell,
		tags:         tags,
		existingTags: existingTags,
	}, nil
}

func requireCommand(runner Runner, name string) (string, error) {
	path, err := runner.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("required command %s was not found: %w", name, err)
	}
	return path, nil
}

func nonemptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

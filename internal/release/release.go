package release

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Options supplies host and process dependencies to the release workflow.
type Options struct {
	Dir       string
	HostOS    string
	In        io.Reader
	Out       io.Writer
	Runner    Runner
	MkTemp    func(dir, pattern string) (string, error)
	RemoveAll func(path string) error
}

// Execute runs the complete interactive release workflow.
func Execute(ctx context.Context, opts Options) error {
	if err := validateOptions(opts); err != nil {
		return err
	}
	fmt.Fprintln(opts.Out, "Checking release environment...")
	env, err := preflight(ctx, opts.Runner, opts.Dir, opts.HostOS)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "Release commit:  %s\n\n", env.head)

	prompter := NewPrompter(opts.In, opts.Out)
	version, err := prompter.SelectVersion(LatestVersion(env.tags), env.existingTags)
	if err != nil {
		return err
	}
	if err := runGoValidation(ctx, opts.Runner, env, opts.HostOS, opts.Out); err != nil {
		return err
	}

	artifactDir, err := opts.MkTemp("", "jb-release-"+version.String()+"-")
	if err != nil {
		return fmt.Errorf("create release artifact directory: %w", err)
	}
	fmt.Fprintf(opts.Out, "Preparing %s artifacts in %s...\n", version, artifactDir)
	assets, err := prepareArtifacts(ctx, opts.Runner, env, version, opts.HostOS, artifactDir)
	if err != nil {
		return fmt.Errorf("%w\nArtifacts preserved at: %s", err, artifactDir)
	}
	if err := revalidateReleaseState(ctx, opts.Runner, env); err != nil {
		return fmt.Errorf("%w\nArtifacts preserved at: %s", err, artifactDir)
	}

	printReleaseSummary(opts.Out, version, env.head, assets)
	confirmed, err := prompter.Confirm()
	if err != nil {
		fmt.Fprintf(opts.Out, "Artifacts preserved at: %s\n", artifactDir)
		return err
	}
	if !confirmed {
		fmt.Fprintf(opts.Out, "Release cancelled. Artifacts preserved at: %s\n", artifactDir)
		return ErrCancelled
	}

	fmt.Fprintln(opts.Out, "Creating tag and publishing GitHub release...")
	url, err := publish(ctx, opts.Runner, publication{
		env:         env,
		version:     version,
		artifactDir: artifactDir,
		out:         opts.Out,
	})
	if err != nil {
		return err
	}
	if err := opts.RemoveAll(artifactDir); err != nil {
		return fmt.Errorf("release published at %s, but temporary artifacts could not be removed from %s: %w", url, artifactDir, err)
	}
	fmt.Fprintf(opts.Out, "Published %s: %s\n", version, url)
	return nil
}

func validateOptions(opts Options) error {
	switch {
	case opts.Dir == "":
		return errors.New("release working directory is required")
	case opts.HostOS == "":
		return errors.New("release host OS is required")
	case opts.In == nil:
		return errors.New("release input is required")
	case opts.Out == nil:
		return errors.New("release output is required")
	case opts.Runner == nil:
		return errors.New("release command runner is required")
	case opts.MkTemp == nil:
		return errors.New("release temporary-directory function is required")
	case opts.RemoveAll == nil:
		return errors.New("release cleanup function is required")
	default:
		return nil
	}
}

func runGoValidation(ctx context.Context, runner Runner, env environment, hostOS string, out io.Writer) error {
	buildOutput := "/dev/null"
	if hostOS == "windows" {
		buildOutput = "NUL"
	}
	commands := []struct {
		label string
		args  []string
	}{
		{label: "go test", args: []string{"test", "./..."}},
		{label: "go vet", args: []string{"vet", "./..."}},
		{label: "go build", args: []string{"build", "-o", buildOutput, "./cmd/jb"}},
	}
	for _, command := range commands {
		fmt.Fprintf(out, "Running %s...\n", command.label)
		if _, err := runner.Run(ctx, env.root, env.goExe, command.args...); err != nil {
			return fmt.Errorf("%s failed: %w", command.label, err)
		}
	}
	return nil
}

func printReleaseSummary(out io.Writer, version Version, head string, assets []string) {
	fmt.Fprintf(out, "\nSelected version: %s\n", version)
	fmt.Fprintf(out, "Release commit:  %s\n", head)
	fmt.Fprintln(out, "Release assets:")
	for _, asset := range assets {
		fmt.Fprintf(out, "  - %s\n", asset)
	}
	fmt.Fprintln(out, "GitHub will generate release notes.")
	fmt.Fprintln(out, "The release will be published immediately after confirmation.")
}

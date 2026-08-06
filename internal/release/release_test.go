package release

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type workflowFixture struct {
	options     Options
	runner      *fakeRunner
	artifactDir string
	removed     *bool
}

func newWorkflowFixture(t *testing.T, input string) workflowFixture {
	t.Helper()
	root := t.TempDir()
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner := successfulPreflightRunner(root)
	goExe := runner.paths["go"]
	runner.responses[commandKey(goExe, "test", "./...")] = fakeResponse{}
	runner.responses[commandKey(goExe, "vet", "./...")] = fakeResponse{}
	runner.responses[commandKey(goExe, "build", "-o", "NUL", "./cmd/jb")] = fakeResponse{}
	shell := runner.paths["pwsh"]
	build := filepath.Join(root, "scripts", "build-local.ps1")
	check := filepath.Join(root, "scripts", "check-artifacts.ps1")
	runner.responses[commandKey(shell, "-NoProfile", "-File", build, "-Version", "v1.2.4", "-OutputDir", artifactDir)] = fakeResponse{}
	runner.responses[commandKey(shell, "-NoProfile", "-File", check, "-Version", "v1.2.4", "-ArtifactDir", artifactDir)] = fakeResponse{}
	configurePublicationResponses(runner, environment{
		root: root,
		head: "abc123",
		git:  runner.paths["git"],
		gh:   runner.paths["gh"],
	}, Version{Major: 1, Minor: 2, Patch: 4}, artifactDir)
	removed := false
	return workflowFixture{
		options: Options{
			Dir:    root,
			HostOS: "windows",
			In:     strings.NewReader(input),
			Out:    io.Discard,
			Runner: runner,
			MkTemp: func(_, _ string) (string, error) { return artifactDir, nil },
			RemoveAll: func(path string) error {
				removed = true
				return os.RemoveAll(path)
			},
		},
		runner:      runner,
		artifactDir: artifactDir,
		removed:     &removed,
	}
}

func configurePublicationResponses(runner *fakeRunner, env environment, version Version, artifactDir string) {
	createArgs := []string{
		"release", "create", version.String(), "--verify-tag", "--generate-notes", "--draft", "--title", version.String(),
	}
	for _, name := range expectedAssets {
		createArgs = append(createArgs, filepath.Join(artifactDir, name))
	}
	runner.responses[commandKey(env.git, "tag", "-a", version.String(), env.head, "-m", "Release "+version.String())] = fakeResponse{}
	runner.responses[commandKey(env.git, "push", "origin", "refs/tags/"+version.String())] = fakeResponse{}
	runner.responses[commandKey(env.gh, createArgs...)] = fakeResponse{}
	runner.responses[commandKey(env.gh, "release", "edit", version.String(), "--draft=false")] = fakeResponse{}
	viewKey := commandKey(env.gh, "release", "view", version.String(), "--json", "isDraft,url,assets")
	if runner.queues == nil {
		runner.queues = make(map[string][]fakeResponse)
	}
	runner.queues[viewKey] = []fakeResponse{
		{output: releaseJSON(true, expectedAssets)},
		{output: releaseJSON(false, expectedAssets)},
	}
}

func TestExecuteDoesNotMutateRemoteBeforeConfirmation(t *testing.T) {
	fixture := newWorkflowFixture(t, "1\nno\n")
	err := Execute(context.Background(), fixture.options)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Execute() error = %v, want ErrCancelled", err)
	}
	for _, call := range fixture.runner.calls {
		if isRemoteMutation(call) {
			t.Fatalf("remote mutation before confirmation: %s", call)
		}
	}
	if *fixture.removed {
		t.Fatal("artifacts were removed after cancellation")
	}
}

func TestExecutePublishesAndCleansArtifacts(t *testing.T) {
	fixture := newWorkflowFixture(t, "1\nyes\n")
	out := new(bytes.Buffer)
	fixture.options.Out = out
	if err := Execute(context.Background(), fixture.options); err != nil {
		t.Fatal(err)
	}
	if !*fixture.removed {
		t.Fatal("successful release did not remove temporary artifacts")
	}
	if _, err := os.Stat(fixture.artifactDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("artifact directory still exists: %v", err)
	}
	for _, text := range []string{
		"Selected version: v1.2.4",
		"Release commit:  abc123",
		"GitHub will generate release notes",
		"will be published immediately",
		expectedAssets[0],
		expectedAssets[7],
		releaseURL,
	} {
		if !strings.Contains(out.String(), text) {
			t.Errorf("output missing %q:\n%s", text, out)
		}
	}
}

func TestExecuteNoTagsOffersFirstPatchVersion(t *testing.T) {
	fixture := newWorkflowFixture(t, "5\n")
	git := fixture.runner.paths["git"]
	fixture.runner.responses[commandKey(git, "tag", "--list")] = fakeResponse{output: "notes\n"}
	out := new(bytes.Buffer)
	fixture.options.Out = out
	err := Execute(context.Background(), fixture.options)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Patch   v0.0.1") {
		t.Fatalf("first patch choice missing:\n%s", out)
	}
}

func TestExecuteStopsAfterFailedGoValidation(t *testing.T) {
	fixture := newWorkflowFixture(t, "1\nyes\n")
	goExe := fixture.runner.paths["go"]
	fixture.runner.responses[commandKey(goExe, "test", "./...")] = fakeResponse{err: errors.New("tests failed")}
	err := Execute(context.Background(), fixture.options)
	if err == nil || !strings.Contains(err.Error(), "go test") {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, call := range fixture.runner.calls {
		if strings.Contains(call, "build-local") || isRemoteMutation(call) {
			t.Fatalf("later phase ran after failed tests: %s", call)
		}
	}
}

func TestExecuteBuildValidationDoesNotWriteIntoRepository(t *testing.T) {
	fixture := newWorkflowFixture(t, "1\nno\n")
	if err := Execute(context.Background(), fixture.options); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Execute() error = %v, want ErrCancelled", err)
	}
	for _, call := range fixture.runner.calls {
		if strings.Contains(call, "go\x00build\x00./cmd/jb") {
			t.Fatalf("repository-writing build command was used: %s", call)
		}
	}
}

func TestExecutePreservesArtifactsAfterPublicationFailure(t *testing.T) {
	fixture := newWorkflowFixture(t, "1\nyes\n")
	gh := fixture.runner.paths["gh"]
	fixture.runner.responses[commandKey(gh, "release", "edit", "v1.2.4", "--draft=false")] = fakeResponse{err: errors.New("GitHub failed")}
	err := Execute(context.Background(), fixture.options)
	if err == nil || !strings.Contains(err.Error(), fixture.artifactDir) {
		t.Fatalf("Execute() error = %v", err)
	}
	if *fixture.removed {
		t.Fatal("artifacts were removed after publication failure")
	}
	if _, err := os.Stat(fixture.artifactDir); err != nil {
		t.Fatalf("artifact directory was not preserved: %v", err)
	}
}

func isRemoteMutation(call string) bool {
	return strings.Contains(call, "git\x00tag\x00-a\x00") ||
		strings.Contains(call, "git\x00push\x00") ||
		strings.Contains(call, "gh\x00release\x00create") ||
		strings.Contains(call, "gh\x00release\x00edit") ||
		strings.Contains(call, "gh\x00release\x00upload")
}

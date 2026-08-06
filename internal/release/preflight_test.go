package release

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type fakeResponse struct {
	output string
	err    error
}

type fakeRunner struct {
	paths     map[string]string
	responses map[string]fakeResponse
	queues    map[string][]fakeResponse
	calls     []string
}

func (r *fakeRunner) LookPath(file string) (string, error) {
	path, found := r.paths[file]
	if !found {
		return "", fmt.Errorf("%s not found", file)
	}
	return path, nil
}

func (r *fakeRunner) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	key := commandKey(name, args...)
	r.calls = append(r.calls, dir+"\x00"+key)
	if queue := r.queues[key]; len(queue) > 0 {
		response := queue[0]
		r.queues[key] = queue[1:]
		return response.output, response.err
	}
	response, found := r.responses[key]
	if !found {
		return "", fmt.Errorf("unexpected command: %s", key)
	}
	return response.output, response.err
}

func commandKey(name string, args ...string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	return strings.Join(append([]string{base}, args...), "\x00")
}

func successfulPreflightRunner(root string) *fakeRunner {
	paths := map[string]string{
		"git":  filepath.Join(root, "bin", "git.exe"),
		"go":   filepath.Join(root, "bin", "go.exe"),
		"gh":   filepath.Join(root, "bin", "gh.exe"),
		"pwsh": filepath.Join(root, "bin", "pwsh.exe"),
		"bash": filepath.Join(root, "bin", "bash.exe"),
	}
	return &fakeRunner{
		paths: paths,
		responses: map[string]fakeResponse{
			commandKey(paths["git"], "rev-parse", "--show-toplevel"):      {output: root + "\n"},
			commandKey(paths["gh"], "auth", "status"):                     {output: "authenticated\n"},
			commandKey(paths["git"], "symbolic-ref", "--short", "HEAD"):   {output: "main\n"},
			commandKey(paths["git"], "status", "--porcelain"):             {output: ""},
			commandKey(paths["git"], "remote", "get-url", "origin"):       {output: "git@github.com:zarxor/scripts.git\n"},
			commandKey(paths["git"], "fetch", "origin", "main", "--tags"): {output: ""},
			commandKey(paths["git"], "rev-parse", "HEAD"):                 {output: "abc123\n"},
			commandKey(paths["git"], "rev-parse", "origin/main"):          {output: "abc123\n"},
			commandKey(paths["git"], "tag", "--list"):                     {output: "notes\nv1.2.3\n"},
		},
	}
}

func TestPreflightReturnsValidatedEnvironment(t *testing.T) {
	root := t.TempDir()
	runner := successfulPreflightRunner(root)
	env, err := preflight(context.Background(), runner, root, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if env.root != root || env.head != "abc123" || env.shell != runner.paths["pwsh"] {
		t.Fatalf("environment = %+v", env)
	}
	if got := LatestVersion(env.tags).String(); got != "v1.2.3" {
		t.Fatalf("latest version = %s, want v1.2.3", got)
	}
	if _, found := env.existingTags["notes"]; !found {
		t.Fatal("existingTags does not contain notes")
	}
}

func TestPreflightUsesBashOutsideWindows(t *testing.T) {
	root := t.TempDir()
	runner := successfulPreflightRunner(root)
	env, err := preflight(context.Background(), runner, root, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if env.shell != runner.paths["bash"] {
		t.Fatalf("shell = %q, want %q", env.shell, runner.paths["bash"])
	}
}

func TestPreflightRejectsUnsafeRepositoryStates(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*fakeRunner)
	}{
		{
			name: "failed authentication", want: "GitHub authentication",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["gh"], "auth", "status")] = fakeResponse{err: errors.New("not logged in")}
			},
		},
		{
			name: "wrong branch", want: "branch main",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "symbolic-ref", "--short", "HEAD")] = fakeResponse{output: "feature\n"}
			},
		},
		{
			name: "dirty tree", want: "working tree is not clean",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "status", "--porcelain")] = fakeResponse{output: " M README.md\n"}
			},
		},
		{
			name: "missing origin", want: "remote origin",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "remote", "get-url", "origin")] = fakeResponse{err: errors.New("missing")}
			},
		},
		{
			name: "failed fetch", want: "fetch origin/main",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "fetch", "origin", "main", "--tags")] = fakeResponse{err: errors.New("offline")}
			},
		},
		{
			name: "stale main", want: "does not match origin/main",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "rev-parse", "origin/main")] = fakeResponse{output: "different\n"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := successfulPreflightRunner(root)
			test.mutate(runner)
			_, err := preflight(context.Background(), runner, root, "windows")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("preflight() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPreflightRejectsMissingDependency(t *testing.T) {
	root := t.TempDir()
	runner := successfulPreflightRunner(root)
	delete(runner.paths, "gh")
	_, err := preflight(context.Background(), runner, root, "windows")
	if err == nil || !strings.Contains(err.Error(), "required command gh") {
		t.Fatalf("preflight() error = %v", err)
	}
}

func TestRevalidateReleaseStateRejectsDrift(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*fakeRunner)
	}{
		{
			name: "branch changed", want: "branch main",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "symbolic-ref", "--short", "HEAD")] = fakeResponse{output: "feature\n"}
			},
		},
		{
			name: "tree changed", want: "working tree changed",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "status", "--porcelain")] = fakeResponse{output: " M README.md\n"}
			},
		},
		{
			name: "head changed", want: "HEAD changed",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "rev-parse", "HEAD")] = fakeResponse{output: "changed\n"}
			},
		},
		{
			name: "remote changed", want: "origin/main changed",
			mutate: func(r *fakeRunner) {
				r.responses[commandKey(r.paths["git"], "rev-parse", "origin/main")] = fakeResponse{output: "changed\n"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := successfulPreflightRunner(root)
			test.mutate(runner)
			env := environment{root: root, head: "abc123", git: runner.paths["git"]}
			err := revalidateReleaseState(context.Background(), runner, env)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("revalidateReleaseState() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

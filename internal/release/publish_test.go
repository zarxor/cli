package release

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseURL = "https://github.com/zarxor/scripts/releases/tag/v1.0.0"

func releaseJSON(draft bool, names []string) string {
	assets := make([]string, 0, len(names))
	for _, name := range names {
		assets = append(assets, `{"name":"`+name+`"}`)
	}
	return `{"isDraft":` + map[bool]string{true: "true", false: "false"}[draft] +
		`,"url":"` + releaseURL + `","assets":[` + strings.Join(assets, ",") + `]}`
}

func successfulPublishRunner(t *testing.T, artifactDir string) (*fakeRunner, publication) {
	t.Helper()
	root := t.TempDir()
	git := filepath.Join(root, "bin", "git.exe")
	gh := filepath.Join(root, "bin", "gh.exe")
	version := Version{Major: 1}
	createArgs := []string{
		"release", "create", version.String(), "--verify-tag", "--generate-notes", "--draft", "--title", version.String(),
	}
	for _, name := range expectedAssets {
		createArgs = append(createArgs, filepath.Join(artifactDir, name))
	}
	viewKey := commandKey(gh, "release", "view", version.String(), "--json", "isDraft,url,assets")
	runner := &fakeRunner{
		responses: map[string]fakeResponse{
			commandKey(git, "tag", "-a", version.String(), "abc123", "-m", "Release "+version.String()): {},
			commandKey(git, "push", "origin", "refs/tags/"+version.String()):                            {},
			commandKey(gh, createArgs...):                                        {},
			commandKey(gh, "release", "edit", version.String(), "--draft=false"): {},
		},
		queues: map[string][]fakeResponse{
			viewKey: {
				{output: releaseJSON(true, expectedAssets)},
				{output: releaseJSON(false, expectedAssets)},
			},
		},
	}
	return runner, publication{
		env:         environment{root: root, head: "abc123", git: git, gh: gh},
		version:     version,
		artifactDir: artifactDir,
	}
}

func TestPublishTagsUploadsVerifiesAndPublishes(t *testing.T) {
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner, request := successfulPublishRunner(t, artifactDir)
	url, err := publish(context.Background(), runner, request)
	if err != nil {
		t.Fatal(err)
	}
	if url != releaseURL {
		t.Fatalf("publish() URL = %q, want %q", url, releaseURL)
	}
	wantOrder := []string{
		"git\x00tag\x00-a\x00v1.0.0\x00abc123\x00-m\x00Release v1.0.0",
		"git\x00push\x00origin\x00refs/tags/v1.0.0",
		"gh\x00release\x00create\x00v1.0.0",
		"gh\x00release\x00view\x00v1.0.0",
		"gh\x00release\x00edit\x00v1.0.0\x00--draft=false",
		"gh\x00release\x00view\x00v1.0.0",
	}
	position := 0
	for _, call := range runner.calls {
		if position < len(wantOrder) && strings.Contains(call, wantOrder[position]) {
			position++
		}
	}
	if position != len(wantOrder) {
		t.Fatalf("calls did not contain expected order at item %d:\n%v", position, runner.calls)
	}
}

func TestPublishReportsEachCompletedExternalStep(t *testing.T) {
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner, request := successfulPublishRunner(t, artifactDir)
	out := new(bytes.Buffer)
	request.out = out
	if _, err := publish(context.Background(), runner, request); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		"Created local tag v1.0.0",
		"Pushed tag v1.0.0 to origin",
		"Created draft release and uploaded 8 assets",
		"Verified draft release assets",
		"Published release v1.0.0",
		"Verified published release",
	} {
		if !strings.Contains(out.String(), text) {
			t.Errorf("publication output missing %q:\n%s", text, out)
		}
	}
}

func TestPublishErrorReportsCompletedSteps(t *testing.T) {
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner, request := successfulPublishRunner(t, artifactDir)
	pushKey := commandKey(request.env.git, "push", "origin", "refs/tags/"+request.version.String())
	runner.responses[pushKey] = fakeResponse{err: errors.New("push failed")}
	_, err := publish(context.Background(), runner, request)
	if err == nil || !strings.Contains(err.Error(), "Completed steps: local tag v1.0.0") {
		t.Fatalf("publish() error = %v", err)
	}
}

func TestPublishRejectsIncorrectDraftAssetsBeforePublishing(t *testing.T) {
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner, request := successfulPublishRunner(t, artifactDir)
	viewKey := commandKey(request.env.gh, "release", "view", request.version.String(), "--json", "isDraft,url,assets")
	runner.queues[viewKey] = []fakeResponse{{output: releaseJSON(true, expectedAssets[:7])}}
	_, err := publish(context.Background(), runner, request)
	if err == nil || !strings.Contains(err.Error(), "missing release asset") {
		t.Fatalf("publish() error = %v", err)
	}
	assertNoCallContains(t, runner.calls, "release\x00edit")
	if !strings.Contains(err.Error(), "gh release upload v1.0.0") {
		t.Fatalf("recovery guidance = %v", err)
	}
}

func TestPublishQuotesMissingAssetRecoveryPaths(t *testing.T) {
	artifactDir := filepath.Join(t.TempDir(), "artifacts with spaces")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExpectedArtifacts(t, artifactDir)
	runner, request := successfulPublishRunner(t, artifactDir)
	request.env.hostOS = "windows"
	viewKey := commandKey(request.env.gh, "release", "view", request.version.String(), "--json", "isDraft,url,assets")
	runner.queues[viewKey] = []fakeResponse{{output: releaseJSON(true, expectedAssets[1:])}}
	_, err := publish(context.Background(), runner, request)
	quotedPath := "'" + filepath.Join(artifactDir, expectedAssets[0]) + "'"
	if err == nil || !strings.Contains(err.Error(), "gh release upload v1.0.0 "+quotedPath) {
		t.Fatalf("publish() recovery = %v, want safely quoted path %s", err, quotedPath)
	}
}

func TestPublishReportsRecoveryForRemoteFailures(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*fakeRunner, publication)
		want       string
		forbidCall string
	}{
		{
			name: "local tag", want: "rerun go run ./cmd/release", forbidCall: "git\x00push",
			mutate: func(r *fakeRunner, p publication) {
				key := commandKey(p.env.git, "tag", "-a", p.version.String(), p.env.head, "-m", "Release "+p.version.String())
				r.responses[key] = fakeResponse{err: errors.New("tag failed")}
			},
		},
		{
			name: "tag push", want: "git push origin refs/tags/v1.0.0", forbidCall: "release\x00create",
			mutate: func(r *fakeRunner, p publication) {
				key := commandKey(p.env.git, "push", "origin", "refs/tags/"+p.version.String())
				r.responses[key] = fakeResponse{err: errors.New("push failed")}
			},
		},
		{
			name: "draft creation", want: "gh release view v1.0.0", forbidCall: "release\x00edit",
			mutate: func(r *fakeRunner, p publication) {
				for key := range r.responses {
					if strings.Contains(key, "release\x00create") {
						r.responses[key] = fakeResponse{err: errors.New("upload failed")}
					}
				}
			},
		},
		{
			name: "publish edit", want: "gh release edit v1.0.0 --draft=false",
			mutate: func(r *fakeRunner, p publication) {
				key := commandKey(p.env.gh, "release", "edit", p.version.String(), "--draft=false")
				r.responses[key] = fakeResponse{err: errors.New("publish failed")}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifactDir := t.TempDir()
			writeExpectedArtifacts(t, artifactDir)
			runner, request := successfulPublishRunner(t, artifactDir)
			test.mutate(runner, request)
			_, err := publish(context.Background(), runner, request)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), artifactDir) {
				t.Fatalf("publish() error = %v; want recovery %q and artifact directory", err, test.want)
			}
			if test.forbidCall != "" {
				assertNoCallContains(t, runner.calls, test.forbidCall)
			}
			assertNoCallContains(t, runner.calls, "tag\x00-d")
			assertNoCallContains(t, runner.calls, "push\x00--delete")
			assertNoCallContains(t, runner.calls, "release\x00delete")
		})
	}
}

func TestPublishRejectsReleaseThatRemainsDraft(t *testing.T) {
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner, request := successfulPublishRunner(t, artifactDir)
	viewKey := commandKey(request.env.gh, "release", "view", request.version.String(), "--json", "isDraft,url,assets")
	runner.queues[viewKey] = []fakeResponse{
		{output: releaseJSON(true, expectedAssets)},
		{output: releaseJSON(true, expectedAssets)},
	}
	_, err := publish(context.Background(), runner, request)
	if err == nil || !strings.Contains(err.Error(), "still a draft") {
		t.Fatalf("publish() error = %v", err)
	}
}

func assertNoCallContains(t *testing.T, calls []string, text string) {
	t.Helper()
	for _, call := range calls {
		if strings.Contains(call, text) {
			t.Fatalf("unexpected call containing %q: %s", text, call)
		}
	}
}

func TestPublishRequiresExactLocalArtifactSet(t *testing.T) {
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	if err := os.Remove(filepath.Join(artifactDir, expectedAssets[0])); err != nil {
		t.Fatal(err)
	}
	runner, request := successfulPublishRunner(t, artifactDir)
	_, err := publish(context.Background(), runner, request)
	if err == nil || !strings.Contains(err.Error(), "missing release asset") {
		t.Fatalf("publish() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("commands ran with incomplete artifacts: %v", runner.calls)
	}
}

package release

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeExpectedArtifacts(t *testing.T, dir string) {
	t.Helper()
	for _, name := range expectedAssets {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateArtifactSetRequiresExactlyExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	writeExpectedArtifacts(t, dir)
	got, err := ValidateArtifactSet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, expectedAssets) {
		t.Fatalf("ValidateArtifactSet() = %v, want %v", got, expectedAssets)
	}
	if err := os.WriteFile(filepath.Join(dir, "unexpected.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateArtifactSet(dir); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unexpected file error = %v", err)
	}
}

func TestValidateArtifactSetRejectsMissingAsset(t *testing.T) {
	dir := t.TempDir()
	writeExpectedArtifacts(t, dir)
	missing := expectedAssets[3]
	if err := os.Remove(filepath.Join(dir, missing)); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateArtifactSet(dir); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing asset error = %v", err)
	}
}

func TestValidateArtifactSetRejectsDirectoryWithExpectedName(t *testing.T) {
	dir := t.TempDir()
	writeExpectedArtifacts(t, dir)
	name := expectedAssets[0]
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateArtifactSet(dir); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestPrepareArtifactsUsesPowerShellOnWindows(t *testing.T) {
	root := t.TempDir()
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner := &fakeRunner{
		responses: map[string]fakeResponse{},
	}
	env := environment{root: root, shell: `C:\tools\pwsh.exe`}
	build := filepath.Join(root, "scripts", "build-local.ps1")
	check := filepath.Join(root, "scripts", "check-artifacts.ps1")
	runner.responses[commandKey(env.shell, "-NoProfile", "-File", build, "-Version", "v1.2.3", "-OutputDir", artifactDir)] = fakeResponse{}
	runner.responses[commandKey(env.shell, "-NoProfile", "-File", check, "-Version", "v1.2.3", "-ArtifactDir", artifactDir)] = fakeResponse{}

	assets, err := prepareArtifacts(context.Background(), runner, env, Version{1, 2, 3}, "windows", artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(assets, expectedAssets) || len(runner.calls) != 2 {
		t.Fatalf("assets = %v, calls = %v", assets, runner.calls)
	}
}

func TestPrepareArtifactsUsesBashOnUnix(t *testing.T) {
	root := t.TempDir()
	artifactDir := t.TempDir()
	writeExpectedArtifacts(t, artifactDir)
	runner := &fakeRunner{
		responses: map[string]fakeResponse{},
	}
	env := environment{root: root, shell: "/usr/bin/bash"}
	build := filepath.Join(root, "scripts", "build-local.sh")
	check := filepath.Join(root, "scripts", "check-artifacts.sh")
	runner.responses[commandKey(env.shell, build, "--version", "v1.2.3", "--output-dir", artifactDir)] = fakeResponse{}
	runner.responses[commandKey(env.shell, check, "--version", "v1.2.3", "--artifact-dir", artifactDir)] = fakeResponse{}

	assets, err := prepareArtifacts(context.Background(), runner, env, Version{1, 2, 3}, "darwin", artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(assets, expectedAssets) || len(runner.calls) != 2 {
		t.Fatalf("assets = %v, calls = %v", assets, runner.calls)
	}
}

func TestExpectedAssetsIncludeMacOSArchives(t *testing.T) {
	for _, name := range []string{
		"jb_darwin_amd64.tar.gz", "jb_darwin_amd64.tar.gz.sha256",
		"jb_darwin_arm64.tar.gz", "jb_darwin_arm64.tar.gz.sha256",
	} {
		if !slices.Contains(expectedAssets, name) {
			t.Fatalf("expectedAssets missing %s", name)
		}
	}
}

func TestPrepareArtifactsStopsAfterBuilderFailure(t *testing.T) {
	root := t.TempDir()
	artifactDir := t.TempDir()
	env := environment{root: root, shell: "/usr/bin/bash"}
	build := filepath.Join(root, "scripts", "build-local.sh")
	runner := &fakeRunner{responses: map[string]fakeResponse{
		commandKey(env.shell, build, "--version", "v1.2.3", "--output-dir", artifactDir): {err: os.ErrPermission},
	}}
	_, err := prepareArtifacts(context.Background(), runner, env, Version{1, 2, 3}, "linux", artifactDir)
	if err == nil || !strings.Contains(err.Error(), "build release artifacts") {
		t.Fatalf("prepareArtifacts() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %v", runner.calls)
	}
}

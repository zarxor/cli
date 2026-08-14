package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

var expectedAssets = []string{
	"jb_darwin_amd64.tar.gz",
	"jb_darwin_amd64.tar.gz.sha256",
	"jb_darwin_arm64.tar.gz",
	"jb_darwin_arm64.tar.gz.sha256",
	"jb_linux_amd64.tar.gz",
	"jb_linux_amd64.tar.gz.sha256",
	"jb_linux_arm64.tar.gz",
	"jb_linux_arm64.tar.gz.sha256",
	"jb_windows_amd64.zip",
	"jb_windows_amd64.zip.sha256",
	"jb_windows_arm64.zip",
	"jb_windows_arm64.zip.sha256",
}

// ValidateArtifactSet requires exactly the documented release files.
func ValidateArtifactSet(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read artifact directory %s: %w", dir, err)
	}
	actual := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() {
			actual = append(actual, name)
			continue
		}
		return nil, fmt.Errorf("release asset %s is not a regular file", name)
	}
	slices.Sort(actual)
	if !slices.Equal(actual, expectedAssets) {
		for _, expected := range expectedAssets {
			if !slices.Contains(actual, expected) {
				return nil, fmt.Errorf("missing release asset %s", expected)
			}
		}
		for _, name := range actual {
			if !slices.Contains(expectedAssets, name) {
				return nil, fmt.Errorf("unexpected release asset %s", name)
			}
		}
		return nil, fmt.Errorf("release artifact set does not match expected files")
	}
	return slices.Clone(actual), nil
}

func prepareArtifacts(
	ctx context.Context,
	runner Runner,
	env environment,
	version Version,
	hostOS string,
	artifactDir string,
) ([]string, error) {
	var buildArgs, checkArgs []string
	if hostOS == "windows" {
		buildArgs = []string{
			"-NoProfile", "-File", filepath.Join(env.root, "scripts", "build-local.ps1"),
			"-Version", version.String(), "-OutputDir", artifactDir,
		}
		checkArgs = []string{
			"-NoProfile", "-File", filepath.Join(env.root, "scripts", "check-artifacts.ps1"),
			"-Version", version.String(), "-ArtifactDir", artifactDir,
		}
	} else {
		buildArgs = []string{
			filepath.Join(env.root, "scripts", "build-local.sh"),
			"--version", version.String(), "--output-dir", artifactDir,
		}
		checkArgs = []string{
			filepath.Join(env.root, "scripts", "check-artifacts.sh"),
			"--version", version.String(), "--artifact-dir", artifactDir,
		}
	}
	if _, err := runner.Run(ctx, env.root, env.shell, buildArgs...); err != nil {
		return nil, fmt.Errorf("build release artifacts: %w", err)
	}
	if _, err := runner.Run(ctx, env.root, env.shell, checkArgs...); err != nil {
		return nil, fmt.Errorf("check release artifacts: %w", err)
	}
	assets, err := ValidateArtifactSet(artifactDir)
	if err != nil {
		return nil, err
	}
	return assets, nil
}

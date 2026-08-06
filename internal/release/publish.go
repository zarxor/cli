package release

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

type publication struct {
	env         environment
	version     Version
	artifactDir string
}

type releaseState struct {
	IsDraft bool   `json:"isDraft"`
	URL     string `json:"url"`
	Assets  []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

type publishError struct {
	cause       error
	artifactDir string
	recovery    string
}

func (e *publishError) Error() string {
	return fmt.Sprintf("%v\nArtifacts preserved at: %s\nRecovery: %s", e.cause, e.artifactDir, e.recovery)
}

func (e *publishError) Unwrap() error {
	return e.cause
}

func publish(ctx context.Context, runner Runner, request publication) (string, error) {
	assets, err := ValidateArtifactSet(request.artifactDir)
	if err != nil {
		return "", err
	}
	version := request.version.String()
	tagRef := "refs/tags/" + version
	if _, err := runner.Run(
		ctx,
		request.env.root,
		request.env.git,
		"tag", "-a", version, request.env.head, "-m", "Release "+version,
	); err != nil {
		return "", publicationFailure(request, err, "rerun go run ./cmd/release after resolving the local tag error")
	}
	if _, err := runner.Run(ctx, request.env.root, request.env.git, "push", "origin", tagRef); err != nil {
		return "", publicationFailure(request, err, "git push origin "+tagRef)
	}

	createArgs := []string{
		"release", "create", version,
		"--verify-tag", "--generate-notes", "--draft", "--title", version,
	}
	for _, name := range assets {
		createArgs = append(createArgs, filepath.Join(request.artifactDir, name))
	}
	if _, err := runner.Run(ctx, request.env.root, request.env.gh, createArgs...); err != nil {
		return "", publicationFailure(request, err, "gh release view "+version)
	}

	draft, err := readRelease(ctx, runner, request)
	if err != nil {
		return "", publicationFailure(request, err, "gh release view "+version)
	}
	if !draft.IsDraft {
		return "", publicationFailure(request, fmt.Errorf("release %s was published before asset verification", version), "gh release view "+version)
	}
	if err := validateReleaseAssets(draft, assets); err != nil {
		recovery := "gh release view " + version
		if missing := missingReleaseAssets(draft, assets); len(missing) > 0 {
			paths := make([]string, 0, len(missing))
			for _, name := range missing {
				paths = append(paths, filepath.Join(request.artifactDir, name))
			}
			recovery = "gh release upload " + version + " " + strings.Join(paths, " ")
		}
		return "", publicationFailure(request, err, recovery)
	}

	if _, err := runner.Run(ctx, request.env.root, request.env.gh, "release", "edit", version, "--draft=false"); err != nil {
		return "", publicationFailure(request, err, "gh release edit "+version+" --draft=false")
	}
	published, err := readRelease(ctx, runner, request)
	if err != nil {
		return "", publicationFailure(request, err, "gh release view "+version)
	}
	if published.IsDraft {
		return "", publicationFailure(request, fmt.Errorf("release %s is still a draft", version), "gh release edit "+version+" --draft=false")
	}
	if err := validateReleaseAssets(published, assets); err != nil {
		return "", publicationFailure(request, err, "gh release view "+version)
	}
	if strings.TrimSpace(published.URL) == "" {
		return "", publicationFailure(request, fmt.Errorf("published release %s has no URL", version), "gh release view "+version)
	}
	return published.URL, nil
}

func publicationFailure(request publication, cause error, recovery string) error {
	return &publishError{
		cause:       cause,
		artifactDir: request.artifactDir,
		recovery:    recovery,
	}
}

func readRelease(ctx context.Context, runner Runner, request publication) (releaseState, error) {
	output, err := runner.Run(
		ctx,
		request.env.root,
		request.env.gh,
		"release", "view", request.version.String(), "--json", "isDraft,url,assets",
	)
	if err != nil {
		return releaseState{}, err
	}
	var state releaseState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		return releaseState{}, fmt.Errorf("decode GitHub release state: %w", err)
	}
	return state, nil
}

func validateReleaseAssets(state releaseState, expected []string) error {
	actual := make([]string, 0, len(state.Assets))
	for _, asset := range state.Assets {
		actual = append(actual, asset.Name)
	}
	slices.Sort(actual)
	if slices.Equal(actual, expected) {
		return nil
	}
	if missing := missingReleaseAssets(state, expected); len(missing) > 0 {
		return fmt.Errorf("missing release asset %s", strings.Join(missing, ", "))
	}
	for _, name := range actual {
		if !slices.Contains(expected, name) {
			return fmt.Errorf("unexpected release asset %s", name)
		}
	}
	return fmt.Errorf("GitHub release assets do not match the local artifact set")
}

func missingReleaseAssets(state releaseState, expected []string) []string {
	actual := make(map[string]struct{}, len(state.Assets))
	for _, asset := range state.Assets {
		actual[asset.Name] = struct{}{}
	}
	var missing []string
	for _, name := range expected {
		if _, found := actual[name]; !found {
			missing = append(missing, name)
		}
	}
	return missing
}

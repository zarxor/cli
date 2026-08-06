package install_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/scripts/internal/adapters"
	"github.com/zarxor/scripts/internal/detect"
	"github.com/zarxor/scripts/internal/install"
	"github.com/zarxor/scripts/internal/platform"
	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/render"
	"github.com/zarxor/scripts/internal/runner"
	"github.com/zarxor/scripts/internal/tools"
)

func TestInstallHonorsInteractiveDeselection(t *testing.T) {
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{selected: []tools.ToolID{profile.Bun}}
	statuses := []install.ToolStatus{
		{Tool: mustTool(t, profile.Git), Selected: true},
		{Tool: mustTool(t, profile.Bun), Selected: true},
	}

	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if summary.Failed {
		t.Fatalf("Run() summary = %#v", summary)
	}
	if got, want := adapter.calls, []string{"install:bun", "verify:bun"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter calls = %v, want %v", got, want)
	}
	if got := resultIDs(summary); !reflect.DeepEqual(got, []tools.ToolID{profile.Bun}) {
		t.Fatalf("result IDs = %v, want [bun]", got)
	}
}

func TestInstallShowsInstalledToolsAsDisabledAndNeverExecutesThem(t *testing.T) {
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{selected: []tools.ToolID{profile.Git, profile.Bun}}
	statuses := []install.ToolStatus{
		{Tool: mustTool(t, profile.Git), Installed: true, CurrentVersion: "2.49.0", Selected: true},
		{Tool: mustTool(t, profile.Bun), Selected: true},
	}

	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if summary.Failed {
		t.Fatalf("Run() summary = %#v", summary)
	}
	if len(selection.items) != 2 {
		t.Fatalf("selection items = %#v, want two", selection.items)
	}
	available := selection.items[0]
	if available.Tool.ID != profile.Bun || available.Disabled || !available.Selected {
		t.Fatalf("first item = %#v, want available Bun", available)
	}
	installed := selection.items[1]
	if installed.Tool.ID != profile.Git {
		t.Fatalf("second item = %#v, want installed Git", installed)
	}
	if !installed.Disabled || installed.Selected {
		t.Fatalf("installed item = %#v, want disabled and unselected", installed)
	}
	for _, want := range []string{"already installed", "2.49.0"} {
		if !strings.Contains(installed.Label, want) {
			t.Fatalf("installed label %q does not contain %q", installed.Label, want)
		}
	}
	if got, want := adapter.calls, []string{"install:bun", "verify:bun"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter calls = %v, want %v", got, want)
	}
}

func TestInstallAllowsAllToolsToBeDeselected(t *testing.T) {
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, nil, install.Options{
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if summary.Failed || len(summary.Results) != 0 || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want empty successful plan", summary, adapter.calls)
	}
}

func TestRunTreatsSelectionCancellationAsSuccessfulNoOp(t *testing.T) {
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{err: render.ErrCancelled}
	var output bytes.Buffer

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git), Selected: true}}, fixtureAdapters(adapter), install.Options{
		Writer:    &output,
		Selection: selection,
	})

	if summary.Failed || !summary.Cancelled || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want successful cancellation", summary, adapter.calls)
	}
	if got, want := output.String(), "Cancelled — no changes made\n"; got != want {
		t.Fatalf("cancellation output = %q, want %q", got, want)
	}
}

func TestRunRendersEveryExecutedResult(t *testing.T) {
	adapter := &fixtureAdapter{}
	var output bytes.Buffer
	statuses := []install.ToolStatus{
		{Tool: mustTool(t, profile.Git)},
		{Tool: mustTool(t, profile.Bun), Installed: true, CurrentVersion: "1.2.0", CandidateVersion: "1.2.0"},
	}
	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &output})
	if summary.Failed {
		t.Fatalf("Run() = %#v", summary)
	}
	if want := "✓ installed Git"; !strings.Contains(output.String(), want) {
		t.Fatalf("output %q does not contain %q", output.String(), want)
	}
	if strings.Contains(output.String(), "✓ up-to-date Bun") {
		t.Fatalf("output %q renders installed Bun as an install result", output.String())
	}
}

func TestRunStopsAfterPostMutationResultRenderingFailure(t *testing.T) {
	wantErr := errors.New("writer failed")
	writer := &failingWriter{failAt: 1, err: wantErr}
	adapter := &fixtureAdapter{}
	statuses := []install.ToolStatus{{Tool: mustTool(t, profile.Git)}, {Tool: mustTool(t, profile.Bun)}}
	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{Yes: true, Writer: writer})
	if !summary.Failed || !reflect.DeepEqual(adapter.calls, []string{"install:git", "verify:git"}) {
		t.Fatalf("Run() = %#v, calls = %v; want stop after first rendered result fails", summary, adapter.calls)
	}
	if len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("results = %#v, want rendering failure", summary.Results)
	}
}

func TestRunKeepsOrdinarySelectionErrorsAsFailures(t *testing.T) {
	wantErr := errors.New("selection failed")
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{err: wantErr}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git), Selected: true}}, fixtureAdapters(adapter), install.Options{
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if !summary.Failed || summary.Cancelled || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want failed selection", summary, adapter.calls)
	}
	if len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("results = %#v, want wrapped selection error", summary.Results)
	}
}

func TestUpdateFiltersMissingToolsAndDefaultsInstalledToolsToSelected(t *testing.T) {
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{selected: []tools.ToolID{profile.Git}}
	statuses := []install.ToolStatus{
		{Tool: mustTool(t, profile.Git), Installed: true, CurrentVersion: "2.48.0", CandidateVersion: "2.49.0"},
		{Tool: mustTool(t, profile.Bun), Installed: false, CandidateVersion: "1.2.0"},
	}

	summary := install.Run(context.Background(), install.Update, statuses, fixtureAdapters(adapter), install.Options{
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if summary.Failed {
		t.Fatalf("Run() summary = %#v", summary)
	}
	if len(selection.items) != 1 || selection.items[0].Tool.ID != profile.Git {
		t.Fatalf("selection items = %#v, want only installed Git", selection.items)
	}
	if !selection.items[0].Selected {
		t.Fatal("installed update item was not selected by default")
	}
	for _, want := range []string{"2.48.0", "2.49.0"} {
		if !strings.Contains(selection.items[0].Label, want) {
			t.Fatalf("selection label %q does not contain %q", selection.items[0].Label, want)
		}
	}
	if got, want := adapter.calls, []string{"update:git", "verify:git"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter calls = %v, want %v", got, want)
	}
}

func TestUpdateWithNoInstalledToolsIsAnEmptyPlan(t *testing.T) {
	adapter := &fixtureAdapter{}

	summary := install.Run(context.Background(), install.Update, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(adapter), install.Options{Writer: &bytes.Buffer{}})

	if summary.Failed || len(summary.Results) != 0 || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want empty successful plan", summary, adapter.calls)
	}
}

func TestYesSkipsInteractiveSelection(t *testing.T) {
	adapter := &fixtureAdapter{}
	selection := &fixtureSelection{err: errors.New("selection should not run")}
	statuses := []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}

	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{
		Yes:       true,
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if summary.Failed || selection.calls != 0 {
		t.Fatalf("Run() = %#v, selection calls = %d", summary, selection.calls)
	}
}

func TestRunWithoutYesOrSelectionDoesNotMutate(t *testing.T) {
	adapter := &fixtureAdapter{}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(adapter), install.Options{Writer: &bytes.Buffer{}})

	if !summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want safe interactive failure", summary, adapter.calls)
	}
}

func TestDryRunRendersWithoutCallingAdapter(t *testing.T) {
	adapter := &fixtureAdapter{}
	var output bytes.Buffer
	statuses := []install.ToolStatus{{
		Tool:             mustTool(t, profile.Git),
		Installed:        true,
		CurrentVersion:   "2.48.0",
		CandidateVersion: "2.49.0",
	}}

	summary := install.Run(context.Background(), install.Update, statuses, fixtureAdapters(adapter), install.Options{
		Yes:    true,
		DryRun: true,
		Writer: &output,
	})

	if summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v", summary, adapter.calls)
	}
	for _, want := range []string{"Git", "2.48.0", "2.49.0", "dry-run"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("dry-run output %q does not contain %q", output.String(), want)
		}
	}
}

func TestRunStopsBeforeMutationWhenPlanRenderingFails(t *testing.T) {
	wantErr := errors.New("writer failed")
	writer := &failingWriter{failAt: 0, err: wantErr}
	adapter := &fixtureAdapter{}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(adapter), install.Options{Yes: true, Writer: writer})

	if !summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want render failure before mutation", summary, adapter.calls)
	}
	if len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("results = %#v, want writer error", summary.Results)
	}
}

func TestRunPropagatesNumberedSelectionRenderingFailure(t *testing.T) {
	wantErr := errors.New("selection writer failed")
	selection := render.NewNumberedSelection(strings.NewReader("\n"), &failingWriter{failAt: 0, err: wantErr})
	adapter := &fixtureAdapter{}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(adapter), install.Options{
		Writer:    &bytes.Buffer{},
		Selection: selection,
	})

	if !summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want selection render failure before mutation", summary, adapter.calls)
	}
	if len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("results = %#v, want selection writer error", summary.Results)
	}
}

func TestDryRunReportsResultRenderingFailure(t *testing.T) {
	wantErr := errors.New("writer failed")
	writer := &failingWriter{failAt: 1, err: wantErr}
	adapter := &fixtureAdapter{}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(adapter), install.Options{Yes: true, DryRun: true, Writer: writer})

	if !summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want dry-run render failure", summary, adapter.calls)
	}
	if len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("results = %#v, want writer error", summary.Results)
	}
}

func TestEmptyPlanReportsVersionTableRenderingFailure(t *testing.T) {
	wantErr := errors.New("writer failed")
	writer := &failingWriter{failAt: 0, err: wantErr}

	summary := install.Run(context.Background(), install.Update, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(&fixtureAdapter{}), install.Options{Writer: writer})

	if !summary.Failed {
		t.Fatalf("Run() = %#v, want empty-plan render failure", summary)
	}
	if len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("results = %#v, want writer error", summary.Results)
	}
}

func TestRunExecutesDuplicateToolsOnlyOnce(t *testing.T) {
	adapter := &fixtureAdapter{}
	git := mustTool(t, profile.Git)
	statuses := []install.ToolStatus{{Tool: git}, {Tool: git}}

	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &bytes.Buffer{}})

	if summary.Failed {
		t.Fatalf("Run() summary = %#v", summary)
	}
	if got, want := adapter.calls, []string{"install:git", "verify:git"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter calls = %v, want %v", got, want)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("results = %#v, want one", summary.Results)
	}
}

func TestUpdateSkipsToolAlreadyAtCandidateVersion(t *testing.T) {
	adapter := &fixtureAdapter{}
	statuses := []install.ToolStatus{{
		Tool:             mustTool(t, profile.Git),
		Installed:        true,
		CurrentVersion:   "2.49.0",
		CandidateVersion: "2.49.0",
	}}

	summary := install.Run(context.Background(), install.Update, statuses, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &bytes.Buffer{}})

	if summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v", summary, adapter.calls)
	}
	if len(summary.Results) != 1 || summary.Results[0].Status != "up-to-date" {
		t.Fatalf("results = %#v, want up-to-date", summary.Results)
	}
}

func TestUpdateSkipsAlreadyCurrentDockerDesktopPackage(t *testing.T) {
	fixture := runner.NewFixture()
	dockerPath := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.LookPaths["docker"] = dockerPath
	fixture.Set(dockerPath, []string{"--version"}, runner.Result{Stdout: "Docker version 28.1.1\n"}, nil)
	fixture.Set("winget", []string{"list", "--id", "Docker.DockerDesktop", "--exact", "--details"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	adapter := adapters.NewWindowsAdapter(fixture, fixture)
	detection, err := adapter.Detect(context.Background(), mustTool(t, profile.Docker))
	if err != nil {
		t.Fatal(err)
	}

	summary := install.Run(context.Background(), install.Update, []install.ToolStatus{{
		Tool:             mustTool(t, profile.Docker),
		Installed:        detection.Installed,
		CurrentVersion:   detection.Current,
		CandidateVersion: detection.Candidate,
	}}, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &bytes.Buffer{}})

	if summary.Failed {
		t.Fatalf("Run() summary = %#v, want already-current Docker Desktop package", summary)
	}
	for _, command := range fixture.Commands {
		if command.Command == "winget" && len(command.Args) > 0 && command.Args[0] == "upgrade" {
			t.Fatalf("already-current Docker Desktop triggered WinGet upgrade: %#v", fixture.Commands)
		}
	}
}

func TestUpdateNormalizesAdapterShapedVersionsBeforeLatestNoOp(t *testing.T) {
	cases := []struct {
		name      string
		current   string
		candidate string
	}{
		{name: "command prefix", current: "git version 2.49.0", candidate: "2.49.0"},
		{name: "Debian epoch and revision", current: "git version 2.48.0", candidate: "1:2.48.0-1"},
		{name: "command suffix", current: "Docker version 28.1.1, build 4eba377", candidate: "28.1.1"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			adapter := &fixtureAdapter{}
			statuses := []install.ToolStatus{{
				Tool:             mustTool(t, profile.Git),
				Installed:        true,
				CurrentVersion:   test.current,
				CandidateVersion: test.candidate,
			}}

			summary := install.Run(context.Background(), install.Update, statuses, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &bytes.Buffer{}})

			if summary.Failed || len(adapter.calls) != 0 {
				t.Fatalf("Run() = %#v, adapter calls = %v", summary, adapter.calls)
			}
			if len(summary.Results) != 1 || summary.Results[0].Status != "up-to-date" {
				t.Fatalf("results = %#v, want normalized up-to-date", summary.Results)
			}
		})
	}
}

func TestRunOrdersDependenciesAndSkipsOnlyFailedDependents(t *testing.T) {
	wantErr := errors.New("dependency failed")
	adapter := &fixtureAdapter{installErrors: map[tools.ToolID]error{"base": wantErr}}
	base := tools.Tool{ID: "base", Name: "Base"}
	dependent := tools.Tool{ID: "dependent", Name: "Dependent", Dependencies: []tools.ToolID{"base"}}
	independent := tools.Tool{ID: "independent", Name: "Independent"}
	statuses := []install.ToolStatus{
		{Tool: dependent},
		{Tool: independent},
		{Tool: base},
	}

	summary := install.Run(context.Background(), install.Install, statuses, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &bytes.Buffer{}})

	if !summary.Failed {
		t.Fatalf("Run() summary = %#v, want failed", summary)
	}
	if got, want := adapter.calls, []string{"install:base", "install:independent", "verify:independent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter calls = %v, want %v", got, want)
	}
	if len(summary.Results) != 3 {
		t.Fatalf("results = %#v, want three", summary.Results)
	}
	if !errors.Is(summary.Results[0].Err, wantErr) || summary.Results[0].Status != "failed" {
		t.Fatalf("base result = %#v, want failed dependency", summary.Results[0])
	}
	if summary.Results[1].Tool.ID != "dependent" || summary.Results[1].Status != "skipped" || summary.Results[1].Err == nil {
		t.Fatalf("dependent result = %#v, want skipped with error", summary.Results[1])
	}
	if summary.Results[2].Tool.ID != "independent" || summary.Results[2].Status != "installed" {
		t.Fatalf("independent result = %#v, want installed", summary.Results[2])
	}
}

func TestRunMarksVerificationFailureInSummary(t *testing.T) {
	wantErr := errors.New("verification failed")
	adapter := &fixtureAdapter{verifyErrors: map[tools.ToolID]error{profile.Git: wantErr}}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, fixtureAdapters(adapter), install.Options{Yes: true, Writer: &bytes.Buffer{}})

	if !summary.Failed || len(summary.Results) != 1 || !errors.Is(summary.Results[0].Err, wantErr) {
		t.Fatalf("Run() summary = %#v, want verification failure", summary)
	}
}

func TestRunRejectsSoleAdapterWhoseOSKeyDoesNotMatchHost(t *testing.T) {
	host, err := platform.Detect()
	if err != nil {
		t.Fatal(err)
	}
	wrongOS := platform.Debian
	if host == wrongOS {
		wrongOS = platform.Arch
	}
	adapter := &fixtureAdapter{}

	summary := install.Run(context.Background(), install.Install, []install.ToolStatus{{Tool: mustTool(t, profile.Git)}}, map[platform.OS]adapters.Adapter{wrongOS: adapter}, install.Options{Yes: true, Writer: &bytes.Buffer{}})

	if !summary.Failed || len(adapter.calls) != 0 {
		t.Fatalf("Run() = %#v, adapter calls = %v; want host/adapter mismatch failure", summary, adapter.calls)
	}
}

type fixtureSelection struct {
	selected []tools.ToolID
	err      error
	items    []install.Item
	calls    int
}

type failingWriter struct {
	writes int
	failAt int
	err    error
}

func (w *failingWriter) Write(value []byte) (int, error) {
	if w.writes >= w.failAt {
		return 0, w.err
	}
	w.writes++
	return len(value), nil
}

func (s *fixtureSelection) Select(_ context.Context, items []install.Item) ([]tools.ToolID, error) {
	s.calls++
	s.items = append([]install.Item(nil), items...)
	return append([]tools.ToolID(nil), s.selected...), s.err
}

type fixtureAdapter struct {
	calls         []string
	installErrors map[tools.ToolID]error
	updateErrors  map[tools.ToolID]error
	verifyErrors  map[tools.ToolID]error
}

func (a *fixtureAdapter) Detect(context.Context, tools.Tool) (detect.Detection, error) {
	return detect.Detection{}, nil
}

func (a *fixtureAdapter) Install(_ context.Context, tool tools.Tool) error {
	a.calls = append(a.calls, fmt.Sprintf("install:%s", tool.ID))
	return a.installErrors[tool.ID]
}

func (a *fixtureAdapter) Update(_ context.Context, tool tools.Tool) error {
	a.calls = append(a.calls, fmt.Sprintf("update:%s", tool.ID))
	return a.updateErrors[tool.ID]
}

func (a *fixtureAdapter) Verify(_ context.Context, tool tools.Tool) error {
	a.calls = append(a.calls, fmt.Sprintf("verify:%s", tool.ID))
	return a.verifyErrors[tool.ID]
}

func fixtureAdapters(adapter adapters.Adapter) map[platform.OS]adapters.Adapter {
	return map[platform.OS]adapters.Adapter{
		platform.Debian:  adapter,
		platform.Arch:    adapter,
		platform.Windows: adapter,
	}
}

func mustTool(t *testing.T, id tools.ToolID) tools.Tool {
	t.Helper()
	tool, ok := tools.Lookup(id)
	if !ok {
		t.Fatalf("unknown fixture tool %q", id)
	}
	return tool
}

func resultIDs(summary install.Summary) []tools.ToolID {
	ids := make([]tools.ToolID, len(summary.Results))
	for i, result := range summary.Results {
		ids[i] = result.Tool.ID
	}
	return ids
}

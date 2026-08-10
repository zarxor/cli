package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/user"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarxor/cli/internal/adapters"
	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/install"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/tools"
)

func TestToolsInstallParsesFlagsForService(t *testing.T) {
	service := &recordingToolsService{}
	err := executeRoot(t, service, "tools", "install", "--profiles= development ", "--only= git , bun ", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}

	want := ToolsRequest{
		Action:       install.Install,
		ProfileNames: []string{"development"},
		Only:         []tools.ToolID{profile.Git, profile.Bun},
		Yes:          true,
		DryRun:       true,
	}
	got := service.requests[0]
	got.Writer = nil
	got.Renderer = nil
	got.Selection = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("service request = %#v, want %#v", got, want)
	}
}

func TestToolsInstallPassesProfileSelectionThroughPlanner(t *testing.T) {
	adapter := newFixtureAdapter()
	service := fixtureService(adapter)
	err := executeRoot(t, service, "tools", "install", "--profiles=development, development", "--only=docker", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}

	want := []tools.ToolID{profile.Docker, profile.DockerBuildx, profile.DockerCompose}
	if got := adapter.detectedIDs(); !sameToolIDs(got, want) {
		t.Fatalf("detected tool IDs = %v, want planner-expanded %v", got, want)
	}
}

func TestToolsUpdateWithoutProfilesScansEverySupportedInstalledTool(t *testing.T) {
	adapter := newFixtureAdapter()
	adapter.detections[profile.Git] = detect.Detection{Installed: true, Current: "2.48.0", Candidate: "2.49.0"}
	service := fixtureService(adapter)
	err := executeRoot(t, service, "tools", "update", "--yes")
	if err != nil {
		t.Fatal(err)
	}

	wantDetected := make([]tools.ToolID, len(tools.Catalog))
	for i, tool := range tools.Catalog {
		wantDetected[i] = tool.ID
	}
	if got := adapter.detectedIDs(); !sameToolIDs(got, wantDetected) {
		t.Fatalf("detected tool IDs = %v, want full catalog %v", got, wantDetected)
	}
	if got, want := adapter.calls, []string{"update:git", "verify:git"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapter calls = %v, want %v", got, want)
	}
}

func TestToolsUpdateOnlyNarrowsLiveScan(t *testing.T) {
	adapter := newFixtureAdapter()
	adapter.detections[profile.Bun] = detect.Detection{Installed: true, Current: "1.1.0", Candidate: "1.2.0"}
	err := executeRoot(t, fixtureService(adapter), "tools", "update", "--only= bun ", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := adapter.detected, []tools.ToolID{profile.Bun}; !reflect.DeepEqual(got, want) {
		t.Fatalf("detected tool IDs = %v, want %v", got, want)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("dry-run adapter mutations = %v, want none", adapter.calls)
	}
}

func TestToolsUpdateShowsOnlyToolsWithAvailableUpdates(t *testing.T) {
	adapter := newFixtureAdapter()
	adapter.detections[profile.Git] = detect.Detection{Installed: true, Current: "2.49.0", Candidate: "2.49.0"}
	adapter.detections[profile.Bun] = detect.Detection{Installed: true, Current: "1.1.0", Candidate: "1.2.0"}
	var output bytes.Buffer
	err := fixtureService(adapter).Run(context.Background(), ToolsRequest{
		Action:   install.Update,
		Only:     []tools.ToolID{profile.Git, profile.Bun},
		Yes:      true,
		DryRun:   true,
		Writer:   &output,
		Renderer: render.NewPlainRenderer(&output),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "Git") {
		t.Fatalf("update output includes an already-current tool: %q", output.String())
	}
	if !strings.Contains(output.String(), "Bun") || !strings.Contains(output.String(), "dry-run") {
		t.Fatalf("update output = %q, want only updateable Bun", output.String())
	}
}

func TestToolsInstallWithoutScopeReachesService(t *testing.T) {
	service := &recordingToolsService{}
	err := executeRoot(t, service, "tools", "install", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("service requests = %d, want one", len(service.requests))
	}
	request := service.requests[0]
	if len(request.ProfileNames) != 0 || len(request.Only) != 0 {
		t.Fatalf("scope = profiles %v, only %v; want empty", request.ProfileNames, request.Only)
	}
}

func TestToolsInstallWithoutScopePlansFullCatalog(t *testing.T) {
	adapter := newFixtureAdapter()
	err := executeRoot(t, fixtureService(adapter), "tools", "install", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	want := make([]tools.ToolID, len(tools.Catalog))
	for index, tool := range tools.Catalog {
		want[index] = tool.ID
	}
	if got := adapter.detectedIDs(); !sameToolIDs(got, want) {
		t.Fatalf("detected tool IDs = %v, want full catalog %v", got, want)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("dry-run adapter mutations = %v, want none", adapter.calls)
	}
}

func TestRequestedToolsReturnsDefensiveCatalogCopy(t *testing.T) {
	planned, err := requestedTools(install.Install, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(planned, tools.Catalog) {
		t.Fatalf("planned tools = %v, want catalog %v", planned, tools.Catalog)
	}
	planned[0].Name = "changed"
	if tools.Catalog[0].Name == "changed" {
		t.Fatal("requestedTools returned the mutable catalog slice")
	}
}

func TestToolsInstallWithoutScopePreselectsFullCatalog(t *testing.T) {
	adapter := newFixtureAdapter()
	selection := &recordingSelection{}
	err := fixtureService(adapter).Run(context.Background(), ToolsRequest{
		Action:    install.Install,
		Writer:    io.Discard,
		Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.items) != len(tools.Catalog) {
		t.Fatalf("selection items = %d, want %d", len(selection.items), len(tools.Catalog))
	}
	for index, item := range selection.items {
		if item.Tool.ID != tools.Catalog[index].ID || !item.Selected {
			t.Fatalf("selection item %d = %#v, want preselected %s", index, item, tools.Catalog[index].ID)
		}
	}
}

func TestToolsServiceReportsDiscoveryAndDetectsInParallel(t *testing.T) {
	adapter := newDetectionProbeAdapter()
	defer adapter.releaseAll()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- fixtureService(adapter).Run(context.Background(), ToolsRequest{
			Action:   install.Install,
			Only:     []tools.ToolID{profile.Git, profile.GitHubCLI},
			Yes:      true,
			DryRun:   true,
			Writer:   &output,
			Renderer: render.NewPlainRenderer(&output),
		})
	}()

	select {
	case <-adapter.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first detection")
	}
	if !strings.Contains(output.String(), "Checking") {
		t.Fatalf("discovery output = %q, want loading feedback before detection completes", output.String())
	}
	select {
	case <-adapter.secondStarted:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("second detection did not start while the first detection was blocked")
	}

	adapter.releaseAll()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "2/2") {
		t.Fatalf("discovery output = %q, want completion feedback", output.String())
	}
	for _, line := range strings.Split(output.String(), "\n") {
		if strings.Contains(line, "Checking installed tools") && (strings.Contains(line, "Git") || strings.Contains(line, "GitHub CLI")) {
			t.Fatalf("discovery progress includes a tool name: %q", line)
		}
	}
}

func TestToolsCommandConstructsAdaptiveSelection(t *testing.T) {
	service := &recordingToolsService{}
	err := executeRoot(t, service, "tools", "install", "--yes", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("service requests = %d, want one", len(service.requests))
	}
	if _, ok := service.requests[0].Selection.(*render.AdaptiveSelection); !ok {
		t.Fatalf("selection = %T, want adaptive selection", service.requests[0].Selection)
	}
}

func TestToolsServiceTreatsSelectionCancellationAsSuccess(t *testing.T) {
	adapter := newFixtureAdapter()
	selection := &recordingSelection{err: render.ErrCancelled}
	err := fixtureService(adapter).Run(context.Background(), ToolsRequest{
		Action:    install.Install,
		Writer:    io.Discard,
		Selection: selection,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want successful cancellation", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter calls = %v, want none", adapter.calls)
	}
}

func TestToolsInstallWithoutScopeYesSkipsSelectionAndInstallsFullCatalog(t *testing.T) {
	adapter := newFixtureAdapter()
	selection := &recordingSelection{err: errors.New("selection should not run")}
	err := fixtureService(adapter).Run(context.Background(), ToolsRequest{
		Action:    install.Install,
		Yes:       true,
		Writer:    io.Discard,
		Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.calls != 0 {
		t.Fatalf("selection calls = %d, want zero", selection.calls)
	}
	want := make([]string, 0, 2*len(tools.Catalog))
	for _, tool := range tools.Catalog {
		want = append(want, "install:"+string(tool.ID), "verify:"+string(tool.ID))
	}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("adapter calls = %v, want full catalog install and verification %v", adapter.calls, want)
	}
}

func TestToolsRejectsEmptyProfileNameBeforeService(t *testing.T) {
	service := &recordingToolsService{}
	err := executeRoot(t, service, "tools", "install", "--profiles=development, ", "--yes")
	if err == nil || !strings.Contains(err.Error(), "profile name cannot be empty") {
		t.Fatalf("error = %v, want empty profile name", err)
	}
	if len(service.requests) != 0 {
		t.Fatalf("service requests = %d, want zero", len(service.requests))
	}
}

func TestToolsRejectsUnknownProfileBeforeAdapterActivity(t *testing.T) {
	adapter := newFixtureAdapter()
	err := executeRoot(t, fixtureService(adapter), "tools", "install", "--profiles=unknown", "--yes")
	if err == nil || !strings.Contains(err.Error(), `unknown profile "unknown"`) {
		t.Fatalf("error = %v, want unknown profile", err)
	}
	if len(adapter.detected) != 0 || len(adapter.calls) != 0 {
		t.Fatalf("adapter activity = detected %v, calls %v; want none", adapter.detected, adapter.calls)
	}
}

func TestToolsReturnsExecutionFailure(t *testing.T) {
	adapter := newFixtureAdapter()
	adapter.installErrors[profile.Git] = errors.New("fixture install failed")
	err := executeRoot(t, fixtureService(adapter), "tools", "install", "--only=git", "--yes")
	if err == nil || !strings.Contains(err.Error(), "fixture install failed") {
		t.Fatalf("error = %v, want execution failure", err)
	}
}

func TestLinuxConfigUsesInvokingSudoUserAndLiveReleaseMetadata(t *testing.T) {
	root := &user.User{Uid: "0", HomeDir: "/root"}
	lookup := func(name string) (*user.User, error) {
		if name != "johan" {
			t.Fatalf("lookup name = %q, want johan", name)
		}
		return &user.User{Username: "johan", Uid: "1000", Gid: "1000", HomeDir: "/home/johan"}, nil
	}

	got, err := linuxConfigFrom("ID=ubuntu\nVERSION_CODENAME=noble\n", "arm64", root, "johan", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Root || got.Home != "/home/johan" || got.InvokingUser != "johan" || got.InvokingUID != 1000 || got.InvokingGID != 1000 || got.Distribution != "ubuntu" || got.Codename != "noble" || got.Architecture != "arm64" {
		t.Fatalf("Linux config = %#v, want root with invoking home and live ubuntu metadata", got)
	}
}

func TestLinuxConfigRejectsUnsupportedArchitecture(t *testing.T) {
	current := &user.User{Uid: "1000", HomeDir: "/home/johan"}
	_, err := linuxConfigFrom("ID=debian\nVERSION_CODENAME=trixie\n", "mips", current, "", nil)
	if err == nil || !strings.Contains(err.Error(), `unsupported Linux architecture "mips"`) {
		t.Fatalf("error = %v, want unsupported architecture", err)
	}
}

func TestLinuxConfigUsesUbuntuCodenameFallback(t *testing.T) {
	current := &user.User{Username: "johan", Uid: "1000", Gid: "1000", HomeDir: "/home/johan"}
	got, err := linuxConfigFrom("ID=linuxmint\nUBUNTU_CODENAME=noble\n", "amd64", current, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Codename != "noble" {
		t.Fatalf("Codename = %q, want Ubuntu fallback noble", got.Codename)
	}
}

func executeRoot(t *testing.T, service ToolsService, args ...string) error {
	t.Helper()
	root := newRootCommand(service)
	root.SetArgs(args)
	root.SetIn(strings.NewReader("\n"))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)
	return root.ExecuteContext(context.Background())
}

type recordingToolsService struct {
	requests []ToolsRequest
	err      error
}

type recordingSelection struct {
	items []install.Item
	err   error
	calls int
}

func (s *recordingSelection) Select(_ context.Context, items []install.Item) ([]tools.ToolID, error) {
	s.calls++
	s.items = append([]install.Item(nil), items...)
	return nil, s.err
}

func (s *recordingToolsService) Run(_ context.Context, request ToolsRequest) error {
	s.requests = append(s.requests, request)
	return s.err
}

type fixtureAdapter struct {
	mu            sync.Mutex
	detected      []tools.ToolID
	detections    map[tools.ToolID]detect.Detection
	detectionErrs map[tools.ToolID]error
	installErrors map[tools.ToolID]error
	updateErrors  map[tools.ToolID]error
	verifyErrors  map[tools.ToolID]error
	calls         []string
}

type detectionProbeAdapter struct {
	mu            sync.Mutex
	started       int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	release       chan struct{}
	firstOnce     sync.Once
	secondOnce    sync.Once
	releaseOnce   sync.Once
}

func newDetectionProbeAdapter() *detectionProbeAdapter {
	return &detectionProbeAdapter{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		release:       make(chan struct{}),
	}
}

func (a *detectionProbeAdapter) Detect(ctx context.Context, _ tools.Tool) (detect.Detection, error) {
	a.mu.Lock()
	a.started++
	started := a.started
	a.mu.Unlock()
	if started == 1 {
		a.firstOnce.Do(func() { close(a.firstStarted) })
	}
	if started == 2 {
		a.secondOnce.Do(func() { close(a.secondStarted) })
	}
	select {
	case <-a.release:
		return detect.Detection{}, nil
	case <-ctx.Done():
		return detect.Detection{}, ctx.Err()
	}
}

func (a *detectionProbeAdapter) Install(context.Context, tools.Tool) error { return nil }

func (a *detectionProbeAdapter) Update(context.Context, tools.Tool) error { return nil }

func (a *detectionProbeAdapter) Verify(context.Context, tools.Tool) error { return nil }

func (a *detectionProbeAdapter) releaseAll() {
	a.releaseOnce.Do(func() { close(a.release) })
}

func newFixtureAdapter() *fixtureAdapter {
	return &fixtureAdapter{
		detections:    make(map[tools.ToolID]detect.Detection),
		detectionErrs: make(map[tools.ToolID]error),
		installErrors: make(map[tools.ToolID]error),
		updateErrors:  make(map[tools.ToolID]error),
		verifyErrors:  make(map[tools.ToolID]error),
	}
}

func (a *fixtureAdapter) Detect(_ context.Context, tool tools.Tool) (detect.Detection, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.detected = append(a.detected, tool.ID)
	return a.detections[tool.ID], a.detectionErrs[tool.ID]
}

func (a *fixtureAdapter) detectedIDs() []tools.ToolID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]tools.ToolID(nil), a.detected...)
}

func (a *fixtureAdapter) Install(_ context.Context, tool tools.Tool) error {
	a.calls = append(a.calls, "install:"+string(tool.ID))
	return a.installErrors[tool.ID]
}

func (a *fixtureAdapter) Update(_ context.Context, tool tools.Tool) error {
	a.calls = append(a.calls, "update:"+string(tool.ID))
	return a.updateErrors[tool.ID]
}

func (a *fixtureAdapter) Verify(_ context.Context, tool tools.Tool) error {
	a.calls = append(a.calls, "verify:"+string(tool.ID))
	return a.verifyErrors[tool.ID]
}

func fixtureService(adapter adapters.Adapter) ToolsService {
	return &toolsService{loadAdapter: func() (adapters.Adapter, error) {
		return adapter, nil
	}}
}

var _ ToolsService = (*recordingToolsService)(nil)
var _ adapters.Adapter = (*fixtureAdapter)(nil)

func sameToolIDs(got, want []tools.ToolID) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[tools.ToolID]int, len(want))
	for _, id := range want {
		counts[id]++
	}
	for _, id := range got {
		counts[id]--
		if counts[id] < 0 {
			return false
		}
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

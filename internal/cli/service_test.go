package cli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zarxor/cli/internal/render"
	backgroundservice "github.com/zarxor/cli/internal/service"
)

func TestServiceInstallDefaultsToT3CodeAndPassesFlags(t *testing.T) {
	service := &recordingServiceService{}
	var output bytes.Buffer
	root := serviceTestRoot(service)
	root.SetArgs([]string{"service", "install", "t3", "--base-dir", "/srv/t3 data", "--dry-run"})
	root.SetOut(&output)
	root.SetErr(io.Discard)

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := ServiceRequest{
		Action:  backgroundservice.Install,
		Name:    "t3-code",
		BaseDir: "/srv/t3 data",
		DryRun:  true,
		Writer:  &output,
	}
	got := service.requests[0]
	got.Writer = &output
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("service request = %#v, want %#v", got, want)
	}
	if !strings.Contains(output.String(), "Would run: fixture t3 service install") || !strings.Contains(output.String(), "fixture output") {
		t.Fatalf("output = %q, want command and service output", output.String())
	}
}

func TestServiceActionsUseT3CodeLifecycle(t *testing.T) {
	for _, action := range []backgroundservice.Action{
		backgroundservice.Update,
		backgroundservice.Status,
		backgroundservice.Uninstall,
		backgroundservice.Start,
		backgroundservice.Stop,
		backgroundservice.Restart,
		backgroundservice.Logs,
		backgroundservice.Repair,
	} {
		t.Run(string(action), func(t *testing.T) {
			service := &recordingServiceService{}
			root := serviceTestRoot(service)
			root.SetArgs([]string{"services", string(action)})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(service.requests) != 1 || service.requests[0].Action != action || service.requests[0].Name != "t3-code" {
				t.Fatalf("requests = %#v, want %s for t3-code", service.requests, action)
			}
		})
	}
}

func TestServiceRejectsUnknownNameBeforeServiceActivity(t *testing.T) {
	service := &recordingServiceService{}
	root := serviceTestRoot(service)
	root.SetArgs([]string{"service", "install", "postgres"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	err := root.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), `unknown service "postgres"`) {
		t.Fatalf("error = %v, want unknown-service error", err)
	}
	if len(service.requests) != 0 {
		t.Fatalf("requests = %#v, want none", service.requests)
	}
}

func TestServiceNameResolvesRegisteredServicesAndAliases(t *testing.T) {
	providers := []serviceProvider{
		{name: "t3-code", aliases: []string{"t3"}},
		{name: "example", aliases: []string{"ex"}},
	}
	for input, want := range map[string]string{
		"example": "example",
		"EX":      "example",
		"t3":      "t3-code",
	} {
		got, err := serviceName([]string{input}, providers)
		if err != nil {
			t.Fatalf("serviceName(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("serviceName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestServiceServiceDispatchesToRegisteredProvider(t *testing.T) {
	manager := &recordingBackgroundManager{}
	service := &serviceService{providers: map[string]serviceProvider{
		"example": {name: "example", loadManager: func() (backgroundservice.Manager, error) { return manager, nil }},
	}}
	_, err := service.Run(context.Background(), ServiceRequest{Name: "example", Action: backgroundservice.Start})
	if err != nil {
		t.Fatal(err)
	}
	if manager.action != backgroundservice.Start {
		t.Fatalf("action = %q, want %q", manager.action, backgroundservice.Start)
	}
}

func serviceTestRoot(service ServiceService) *cobra.Command {
	return newRootCommandWithAllServices(
		serviceTestTools{},
		serviceTestSkills{},
		service,
		func(*cobra.Command) render.Theme {
			return render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})
		},
	)
}

type recordingServiceService struct {
	requests []ServiceRequest
}

type recordingBackgroundManager struct {
	action backgroundservice.Action
}

func (m *recordingBackgroundManager) RunWithOptions(_ context.Context, action backgroundservice.Action, _ string, _ bool, _ backgroundservice.RunOptions) (backgroundservice.Result, error) {
	m.action = action
	return backgroundservice.Result{}, nil
}

func (s *recordingServiceService) Run(_ context.Context, request ServiceRequest) (backgroundservice.Result, error) {
	s.requests = append(s.requests, request)
	return backgroundservice.Result{Command: "fixture t3 service install", Output: "fixture output", DryRun: request.DryRun}, nil
}

type serviceTestTools struct{}

func (serviceTestTools) Run(context.Context, ToolsRequest) error { return nil }

type serviceTestSkills struct{}

func (serviceTestSkills) Run(context.Context, SkillsRequest) error { return nil }

var _ ServiceService = (*recordingServiceService)(nil)
var _ ToolsService = serviceTestTools{}
var _ SkillsService = serviceTestSkills{}

package tools_test

import (
	"reflect"
	"testing"

	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/tools"
)

func TestResolveToolsReturnsRequestedTool(t *testing.T) {
	got, err := tools.ResolveTools([]profile.ToolID{profile.Git})
	if err != nil {
		t.Fatalf("ResolveTools() error = %v", err)
	}

	want := []profile.Tool{{ID: profile.Git, Name: "Git"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveTools() = %#v, want %#v", got, want)
	}
}

func TestResolveToolsRejectsUnknownTool(t *testing.T) {
	_, err := tools.ResolveTools([]profile.ToolID{"does-not-exist"})
	if err == nil {
		t.Fatal("ResolveTools() error = nil, want an unknown-tool error")
	}
}

func TestCatalogIncludesAgentTools(t *testing.T) {
	tests := []struct {
		id   profile.ToolID
		name string
	}{
		{profile.Claude, "Claude Code"},
		{profile.Codex, "Codex"},
		{profile.T3Code, "T3 Code"},
	}
	for _, test := range tests {
		t.Run(string(test.id), func(t *testing.T) {
			tool, ok := tools.Lookup(test.id)
			if !ok || tool.Name != test.name {
				t.Fatalf("Lookup(%q) = %#v, %v; want %q", test.id, tool, ok, test.name)
			}
		})
	}
}

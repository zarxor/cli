package render_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/render"
	"github.com/zarxor/cli/internal/tools"
)

func TestNumberedSelectionTogglesDefaultSelections(t *testing.T) {
	var output bytes.Buffer
	selection := render.NewNumberedSelection(strings.NewReader("2\n"), &output)
	items := []render.Item{
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git", Selected: true},
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
	}

	got, err := selection.Select(context.Background(), items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []tools.ToolID{profile.Git}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() = %v, want %v", got, want)
	}
	for _, want := range []string{"1", "[x]", "Git", "2", "Bun", "Toggle numbers"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("selection output %q does not contain %q", output.String(), want)
		}
	}
}

func TestNumberedSelectionKeepsDefaultsOnEmptyInput(t *testing.T) {
	selection := render.NewNumberedSelection(strings.NewReader("\n"), &bytes.Buffer{})
	items := []render.Item{
		{Tool: tools.Tool{ID: profile.Git}, Selected: true},
		{Tool: tools.Tool{ID: profile.Bun}, Selected: false},
	}

	got, err := selection.Select(context.Background(), items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []tools.ToolID{profile.Git}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() = %v, want %v", got, want)
	}
}

func TestNumberedSelectionGroupsAvailableAndInstalledItems(t *testing.T) {
	var output bytes.Buffer
	selection := render.NewNumberedSelection(strings.NewReader("1\n"), &output)
	items := []render.Item{
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git (already installed: 2.49.0)", Disabled: true},
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
	}

	got, err := selection.Select(context.Background(), items)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Select() = %v, want no selected tools after toggling available Bun", got)
	}
	for _, want := range []string{"Available to install", "1", "Bun", "Already installed", "[-]", "Git (already installed: 2.49.0)"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q does not contain %q", output.String(), want)
		}
	}
	for _, line := range strings.Split(output.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "Git (already installed") && len(trimmed) > 0 && trimmed[0] >= '0' && trimmed[0] <= '9' {
			t.Fatalf("installed tool was assigned a selectable number: %q", line)
		}
	}
}

func TestNumberedSelectionGroupsItemsByCreator(t *testing.T) {
	var output bytes.Buffer
	selection := render.NewNumberedSelection(strings.NewReader("\n"), &output)
	items := []render.Item{
		{Tool: tools.Tool{ID: profile.Git, Name: "code-review"}, Group: "Matt Pocock", Label: "code-review", Selected: true},
		{Tool: tools.Tool{ID: profile.Bun, Name: "tdd"}, Group: "Matt Pocock", Label: "tdd", Selected: true},
		{Tool: tools.Tool{ID: profile.Node, Name: "impeccable"}, Group: "Paul Bakaus", Label: "impeccable", Selected: true},
	}
	if _, err := selection.Select(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{"Matt Pocock", "code-review", "tdd", "Paul Bakaus", "impeccable"} {
		if !strings.Contains(got, want) {
			t.Fatalf("selection output %q does not contain creator group or skill %q", got, want)
		}
	}
	if strings.Index(got, "Matt Pocock") > strings.Index(got, "Paul Bakaus") {
		t.Fatalf("selection output = %q, want catalog creator order", got)
	}
}

func TestNumberedSelectionRejectsClosedInput(t *testing.T) {
	items := []render.Item{{Tool: tools.Tool{ID: profile.Git}, Selected: true}}
	for _, input := range []string{"", "1"} {
		t.Run(input, func(t *testing.T) {
			selection := render.NewNumberedSelection(strings.NewReader(input), &bytes.Buffer{})
			if _, err := selection.Select(context.Background(), items); err == nil {
				t.Fatalf("Select() error = nil for closed input %q", input)
			}
		})
	}
}

func TestNumberedSelectionRejectsMalformedNonblankInput(t *testing.T) {
	for _, input := range []string{",", "1,", ",1", "1,,1"} {
		t.Run(input, func(t *testing.T) {
			selection := render.NewNumberedSelection(strings.NewReader(input+"\n"), &bytes.Buffer{})
			items := []render.Item{{Tool: tools.Tool{ID: profile.Git}, Selected: true}}

			if _, err := selection.Select(context.Background(), items); err == nil {
				t.Fatalf("Select() error = nil for malformed input %q", input)
			}
		})
	}
}

func TestVersionTableAlignsCurrentAndCandidateVersions(t *testing.T) {
	var output bytes.Buffer
	rows := []render.VersionRow{
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, CurrentVersion: "2.48.0", CandidateVersion: "2.49.0"},
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, CurrentVersion: "1.2.0"},
	}

	if err := render.VersionTable(&output, rows); err != nil {
		t.Fatal(err)
	}
	want := "TOOL  CURRENT  CANDIDATE\nGit   2.48.0   2.49.0\nBun   1.2.0    -\n"
	if got := output.String(); got != want {
		t.Fatalf("VersionTable() output = %q, want %q", got, want)
	}
}

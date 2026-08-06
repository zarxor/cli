package render_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/render"
	"github.com/zarxor/scripts/internal/tools"
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

func TestNumberedSelectionCannotToggleDisabledItem(t *testing.T) {
	var output bytes.Buffer
	selection := render.NewNumberedSelection(strings.NewReader("1\n"), &output)
	items := []render.Item{
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git (already installed: 2.49.0)", Disabled: true},
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
	}

	got, err := selection.Select(context.Background(), items)
	if err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("Select() = %v, %v; want disabled-item error", got, err)
	}
	for _, want := range []string{"[-]", "already installed"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q does not contain %q", output.String(), want)
		}
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

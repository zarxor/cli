package render

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/huh/v2"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/tools"
)

func TestInteractiveSelectionMovesTogglesAndAccepts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	input := bytes.NewBufferString("\x1b[B \r")
	var output bytes.Buffer
	selection := NewInteractiveSelection(input, &output, NewTheme(ThemeOptions{Mode: ColorNever, Dark: true}))
	items := selectionFixtureItems()

	got, err := selection.Select(ctx, items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []tools.ToolID{profile.Git}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected IDs = %v, want %v", got, want)
	}
}

func TestInteractiveSelectionReturnsIDsInItemOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	input := bytes.NewBufferString(" \x1b[B \r")
	selection := NewInteractiveSelection(input, &bytes.Buffer{}, NewTheme(ThemeOptions{Mode: ColorNever, Dark: true}))

	got, err := selection.Select(ctx, selectionFixtureItems())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("selected IDs = %v, want none", got)
	}
}

func TestInteractiveSelectionCannotToggleDisabledItem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	input := bytes.NewBufferString("\x1b[B \r")
	selection := NewInteractiveSelection(input, &bytes.Buffer{}, NewTheme(ThemeOptions{Mode: ColorNever, Dark: true}))
	items := []Item{
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git (already installed: 2.49.0)", Disabled: true},
	}

	got, err := selection.Select(ctx, items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []tools.ToolID{profile.Bun}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected IDs = %v, want %v", got, want)
	}
}

func TestInteractiveSelectionMapsControlCToCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	selection := NewInteractiveSelection(bytes.NewBufferString("\x03"), &bytes.Buffer{}, NewTheme(ThemeOptions{Mode: ColorNever, Dark: true}))

	got, err := selection.Select(ctx, selectionFixtureItems())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Select() error = %v, want cancellation", err)
	}
	if got != nil {
		t.Fatalf("selected IDs = %v, want nil", got)
	}
}

func TestInteractiveSelectionMapsEscapeToCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	selection := NewInteractiveSelection(bytes.NewBufferString("\x1b"), &bytes.Buffer{}, NewTheme(ThemeOptions{Mode: ColorNever, Dark: true}))

	got, err := selection.Select(ctx, selectionFixtureItems())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Select() error = %v, want cancellation", err)
	}
	if got != nil {
		t.Fatalf("selected IDs = %v, want nil", got)
	}
}

func TestInteractiveSelectionClassifiesRecoverableTerminalStartupFailure(t *testing.T) {
	wantErr := errors.New("raw mode unavailable")
	selection := newInteractiveSelection(
		bytes.NewBuffer(nil),
		&bytes.Buffer{},
		NewTheme(ThemeOptions{Mode: ColorNever}),
		func(context.Context, *huh.Form) error {
			return errors.New("huh: error making terminal raw: " + wantErr.Error())
		},
	)

	_, err := selection.Select(context.Background(), selectionFixtureItems())
	if !errors.Is(err, ErrInteractiveUnavailable) {
		t.Fatalf("Select() error = %v, want interactive-unavailable", err)
	}
}

func TestInteractiveSelectionPreservesOrdinaryFormFailure(t *testing.T) {
	wantErr := errors.New("output failed")
	selection := newInteractiveSelection(
		bytes.NewBuffer(nil),
		&bytes.Buffer{},
		NewTheme(ThemeOptions{Mode: ColorNever}),
		func(context.Context, *huh.Form) error { return wantErr },
	)

	_, err := selection.Select(context.Background(), selectionFixtureItems())
	if !errors.Is(err, wantErr) || errors.Is(err, ErrInteractiveUnavailable) {
		t.Fatalf("Select() error = %v, want ordinary form failure", err)
	}
}

func TestBoldActiveLabelStylesOnlyCursorRow(t *testing.T) {
	view := "❯ [✓] Git\n  [✓] Bun"

	got := boldActiveLabel(view, "Git")
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[0], "\x1b[1m") || stripANSI(lines[0]) != "❯ [✓] Git" {
		t.Fatalf("active row = %q, want bold Git", lines[0])
	}
	if strings.Contains(lines[1], "\x1b[") || lines[1] != "  [✓] Bun" {
		t.Fatalf("inactive row = %q, want unstyled Bun", lines[1])
	}
}

func TestDisabledInteractiveRowIsMutedAndUsesDisabledMarker(t *testing.T) {
	theme := NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true})
	view := "❯ [✗] Git (already installed: 2.49.0)\n  [✓] Bun"
	labels := map[tools.ToolID]string{
		profile.Git: "Git (already installed: 2.49.0)",
		profile.Bun: "Bun",
	}
	disabled := map[tools.ToolID]bool{profile.Git: true}

	got := grayDisabledRows(view, labels, disabled, theme)
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[0], "\x1b[") || stripANSI(lines[0]) != "❯ [-] Git (already installed: 2.49.0)" {
		t.Fatalf("disabled row = %q, want muted disabled marker and installation info", lines[0])
	}
	if lines[1] != "  [✓] Bun" {
		t.Fatalf("enabled row = %q, want unchanged", lines[1])
	}
}

func TestInteractiveRowsGroupAvailableBeforeInstalled(t *testing.T) {
	theme := NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true})
	view := "❯ [✓] Bun\n  [-] Git (already installed: 2.49.0)"
	labels := map[tools.ToolID]string{
		profile.Bun: "Bun",
		profile.Git: "Git (already installed: 2.49.0)",
	}
	disabled := map[tools.ToolID]bool{profile.Git: true}

	got := groupSelectionRows(view, labels, disabled, theme)
	want := "Available to install\n❯ [✓] Bun\n\nAlready installed\n  [-] Git (already installed: 2.49.0)"
	if plain := stripANSI(got); plain != want {
		t.Fatalf("grouped rows = %q, want %q", plain, want)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("group headings are not styled: %q", got)
	}
}

func selectionFixtureItems() []Item {
	return []Item{
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git", Selected: true},
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
	}
}

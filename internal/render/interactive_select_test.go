package render

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zarxor/scripts/internal/profile"
	"github.com/zarxor/scripts/internal/tools"
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

func selectionFixtureItems() []Item {
	return []Item{
		{Tool: tools.Tool{ID: profile.Git, Name: "Git"}, Label: "Git", Selected: true},
		{Tool: tools.Tool{ID: profile.Bun, Name: "Bun"}, Label: "Bun", Selected: true},
	}
}

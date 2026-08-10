package render

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/tools"
)

func TestAdaptiveSelectionUsesInteractiveForTerminalStreams(t *testing.T) {
	interactive := &selectionStub{ids: []tools.ToolID{profile.Git}}
	fallback := &selectionStub{}
	selection := newAdaptiveSelection(true, interactive, fallback)

	got, err := selection.Select(context.Background(), selectionFixtureItems())
	if err != nil || !reflect.DeepEqual(got, []tools.ToolID{profile.Git}) {
		t.Fatalf("Select() = %v, %v", got, err)
	}
	if interactive.calls != 1 || fallback.calls != 0 {
		t.Fatalf("calls = interactive %d, fallback %d", interactive.calls, fallback.calls)
	}
}

func TestAdaptiveSelectionUsesFallbackForRedirectedStreams(t *testing.T) {
	interactive := &selectionStub{}
	fallback := &selectionStub{ids: []tools.ToolID{profile.Bun}}
	selection := newAdaptiveSelection(false, interactive, fallback)

	got, err := selection.Select(context.Background(), selectionFixtureItems())
	if err != nil || !reflect.DeepEqual(got, []tools.ToolID{profile.Bun}) {
		t.Fatalf("Select() = %v, %v", got, err)
	}
	if interactive.calls != 0 || fallback.calls != 1 {
		t.Fatalf("calls = interactive %d, fallback %d", interactive.calls, fallback.calls)
	}
}

func TestAdaptiveSelectionFallsBackWhenInteractiveIsUnavailable(t *testing.T) {
	interactive := &selectionStub{err: ErrInteractiveUnavailable}
	fallback := &selectionStub{ids: []tools.ToolID{profile.Git}}
	selection := newAdaptiveSelection(true, interactive, fallback)

	got, err := selection.Select(context.Background(), selectionFixtureItems())
	if err != nil || !reflect.DeepEqual(got, []tools.ToolID{profile.Git}) {
		t.Fatalf("Select() = %v, %v", got, err)
	}
	if interactive.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls = interactive %d, fallback %d", interactive.calls, fallback.calls)
	}
}

func TestAdaptiveSelectionDoesNotReplayInputAfterInteractiveError(t *testing.T) {
	wantErr := errors.New("input failed")
	interactive := &selectionStub{err: wantErr}
	fallback := &selectionStub{}
	selection := newAdaptiveSelection(true, interactive, fallback)

	if _, err := selection.Select(context.Background(), selectionFixtureItems()); !errors.Is(err, wantErr) {
		t.Fatalf("Select() error = %v, want %v", err, wantErr)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want zero", fallback.calls)
	}
}

func TestNewAdaptiveSelectionUsesANSIPlainNumberedFallbackForBuffers(t *testing.T) {
	var output bytes.Buffer
	selection := NewAdaptiveSelection(strings.NewReader("\n"), &output, NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true}))

	got, err := selection.Select(context.Background(), selectionFixtureItems())
	if err != nil {
		t.Fatal(err)
	}
	if want := []tools.ToolID{profile.Git, profile.Bun}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected IDs = %v, want %v", got, want)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("numbered fallback contains ANSI: %q", output.String())
	}
}

type selectionStub struct {
	ids   []tools.ToolID
	err   error
	calls int
}

func (s *selectionStub) Select(context.Context, []Item) ([]tools.ToolID, error) {
	s.calls++
	return append([]tools.ToolID(nil), s.ids...), s.err
}

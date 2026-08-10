package render

import (
	"context"
	"errors"
	"io"
)

type AdaptiveSelection struct {
	terminal    bool
	interactive SelectionUI
	fallback    SelectionUI
}

func NewAdaptiveSelection(in io.Reader, out io.Writer, theme Theme) SelectionUI {
	return NewAdaptiveSelectionWithTitle(in, out, theme, "tools")
}

// NewAdaptiveSelectionWithTitle creates the same terminal-aware selector as
// NewAdaptiveSelection, with a resource-specific title for the interactive
// presentation.
func NewAdaptiveSelectionWithTitle(in io.Reader, out io.Writer, theme Theme, title string) SelectionUI {
	inputTTY, outputTTY := streamsAreTerminal(in, out)
	return newAdaptiveSelection(
		inputTTY && outputTTY,
		NewInteractiveSelectionWithTitle(in, out, theme, title),
		NewNumberedSelection(in, out),
	)
}

func newAdaptiveSelection(terminal bool, interactive, fallback SelectionUI) SelectionUI {
	return &AdaptiveSelection{terminal: terminal, interactive: interactive, fallback: fallback}
}

func (s *AdaptiveSelection) Select(ctx context.Context, items []Item) ([]SelectionID, error) {
	if !s.terminal {
		return s.fallback.Select(ctx, items)
	}
	selected, err := s.interactive.Select(ctx, items)
	if errors.Is(err, ErrInteractiveUnavailable) {
		return s.fallback.Select(ctx, items)
	}
	return selected, err
}

var _ SelectionUI = (*AdaptiveSelection)(nil)

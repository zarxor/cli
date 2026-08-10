package render

import (
	"context"
	"errors"
	"io"

	"github.com/zarxor/cli/internal/tools"
)

type AdaptiveSelection struct {
	terminal    bool
	interactive SelectionUI
	fallback    SelectionUI
}

func NewAdaptiveSelection(in io.Reader, out io.Writer, theme Theme) SelectionUI {
	inputTTY, outputTTY := streamsAreTerminal(in, out)
	return newAdaptiveSelection(
		inputTTY && outputTTY,
		NewInteractiveSelection(in, out, theme),
		NewNumberedSelection(in, out),
	)
}

func newAdaptiveSelection(terminal bool, interactive, fallback SelectionUI) SelectionUI {
	return &AdaptiveSelection{terminal: terminal, interactive: interactive, fallback: fallback}
}

func (s *AdaptiveSelection) Select(ctx context.Context, items []Item) ([]tools.ToolID, error) {
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

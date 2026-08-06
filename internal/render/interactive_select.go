package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/zarxor/scripts/internal/tools"
)

type InteractiveSelection struct {
	in      io.Reader
	out     io.Writer
	theme   Theme
	runForm func(context.Context, *huh.Form) error
}

func NewInteractiveSelection(in io.Reader, out io.Writer, theme Theme) SelectionUI {
	return newInteractiveSelection(in, out, theme, func(ctx context.Context, form *huh.Form) error {
		return form.RunWithContext(ctx)
	})
}

func newInteractiveSelection(in io.Reader, out io.Writer, theme Theme, runForm func(context.Context, *huh.Form) error) *InteractiveSelection {
	return &InteractiveSelection{in: in, out: out, theme: theme, runForm: runForm}
}

func (s *InteractiveSelection) Select(ctx context.Context, items []Item) ([]tools.ToolID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	selected := make([]tools.ToolID, 0, len(items))
	options := make([]huh.Option[tools.ToolID], 0, len(items))
	for _, item := range items {
		label := item.Label
		if label == "" {
			label = item.Tool.Name
		}
		options = append(options, huh.NewOption(label, item.Tool.ID).Selected(item.Selected))
		if item.Selected {
			selected = append(selected, item.Tool.ID)
		}
	}

	multiSelect := huh.NewMultiSelect[tools.ToolID]().
		Title("Select tools").
		Description("↑/↓ move   Space toggle   Enter continue   Esc cancel").
		Options(options...).
		Value(&selected).
		Filterable(false)
	field := &interactiveField{MultiSelect: multiSelect, labels: make(map[tools.ToolID]string, len(items))}
	for _, item := range items {
		field.labels[item.Tool.ID] = item.Label
		if field.labels[item.Tool.ID] == "" {
			field.labels[item.Tool.ID] = item.Tool.Name
		}
	}

	keys := huh.NewDefaultKeyMap()
	keys.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "cancel"),
	)
	keys.MultiSelect.Toggle = key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "toggle"),
	)

	form := huh.NewForm(huh.NewGroup(field)).
		WithInput(s.in).
		WithOutput(s.out).
		WithTheme(s.theme.HuhTheme()).
		WithKeyMap(keys).
		WithShowHelp(false)
	err := s.runForm(ctx, form)
	if errors.Is(err, huh.ErrUserAborted) {
		return nil, ErrCancelled
	}
	if interactiveStartupUnavailable(err) {
		return nil, fmt.Errorf("%w: %w", ErrInteractiveUnavailable, err)
	}
	if err != nil {
		return nil, fmt.Errorf("interactive selection: %w", err)
	}

	selectedSet := make(map[tools.ToolID]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	ordered := make([]tools.ToolID, 0, len(selectedSet))
	for _, item := range items {
		if _, ok := selectedSet[item.Tool.ID]; ok {
			ordered = append(ordered, item.Tool.ID)
		}
	}
	return ordered, nil
}

func interactiveStartupUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"error entering raw mode",
		"error making terminal raw",
		"error getting console mode",
		"error setting console mode",
		"error getting terminal state",
		"could not create cancelable reader",
		"could not open tty",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

var _ SelectionUI = (*InteractiveSelection)(nil)

type interactiveField struct {
	*huh.MultiSelect[tools.ToolID]
	labels map[tools.ToolID]string
}

func (f *interactiveField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	updated, command := f.MultiSelect.Update(msg)
	f.MultiSelect = updated.(*huh.MultiSelect[tools.ToolID])
	return f, command
}

func (f *interactiveField) View() string {
	view := f.MultiSelect.View()
	hovered, ok := f.MultiSelect.Hovered()
	if !ok {
		return view
	}
	return boldActiveLabel(view, f.labels[hovered])
}

func (f *interactiveField) WithTheme(theme huh.Theme) huh.Field {
	f.MultiSelect.WithTheme(theme)
	return f
}

func (f *interactiveField) WithKeyMap(keyMap *huh.KeyMap) huh.Field {
	f.MultiSelect.WithKeyMap(keyMap)
	return f
}

func (f *interactiveField) WithWidth(width int) huh.Field {
	f.MultiSelect.WithWidth(width)
	return f
}

func (f *interactiveField) WithHeight(height int) huh.Field {
	f.MultiSelect.WithHeight(height)
	return f
}

func (f *interactiveField) WithPosition(position huh.FieldPosition) huh.Field {
	f.MultiSelect.WithPosition(position)
	return f
}

func boldActiveLabel(view, label string) string {
	if label == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	for index, line := range lines {
		if !strings.Contains(line, "❯") {
			continue
		}
		labelAt := strings.LastIndex(line, label)
		if labelAt < 0 {
			return view
		}
		bold := lipgloss.NewStyle().Bold(true).Render(label)
		lines[index] = line[:labelAt] + bold + line[labelAt+len(label):]
		return strings.Join(lines, "\n")
	}
	return view
}

var _ huh.Field = (*interactiveField)(nil)

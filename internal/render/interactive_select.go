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
	"github.com/charmbracelet/x/ansi"
)

type InteractiveSelection struct {
	in      io.Reader
	out     io.Writer
	theme   Theme
	title   string
	runForm func(context.Context, *huh.Form) error
}

func NewInteractiveSelection(in io.Reader, out io.Writer, theme Theme) SelectionUI {
	return NewInteractiveSelectionWithTitle(in, out, theme, "tools")
}

// NewInteractiveSelectionWithTitle creates an interactive selector with a
// resource-specific title.
func NewInteractiveSelectionWithTitle(in io.Reader, out io.Writer, theme Theme, title string) SelectionUI {
	return newInteractiveSelectionWithTitle(in, out, theme, func(ctx context.Context, form *huh.Form) error {
		return form.RunWithContext(ctx)
	}, title)
}

func newInteractiveSelection(in io.Reader, out io.Writer, theme Theme, runForm func(context.Context, *huh.Form) error) *InteractiveSelection {
	return newInteractiveSelectionWithTitle(in, out, theme, runForm, "tools")
}

func newInteractiveSelectionWithTitle(in io.Reader, out io.Writer, theme Theme, runForm func(context.Context, *huh.Form) error, title string) *InteractiveSelection {
	return &InteractiveSelection{in: in, out: out, theme: theme, title: title, runForm: runForm}
}

func (s *InteractiveSelection) Select(ctx context.Context, items []Item) ([]SelectionID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	selected := make([]SelectionID, 0, len(items))
	options := make([]huh.Option[SelectionID], 0, len(items))
	for _, item := range items {
		label := item.Label
		if label == "" {
			label = itemName(item)
		}
		id := itemID(item)
		options = append(options, huh.NewOption(label, id).Selected(item.Selected && !item.Disabled))
		if item.Selected && !item.Disabled {
			selected = append(selected, id)
		}
	}

	title := s.title
	if title == "" {
		title = "tools"
	}
	multiSelect := huh.NewMultiSelect[SelectionID]().
		Title("Select " + title).
		Description("↑/↓ move   Space toggle   Enter continue   Esc cancel").
		Options(options...).
		Value(&selected).
		Filterable(false)
	keys := huh.NewDefaultKeyMap()
	keys.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "cancel"),
	)
	keys.MultiSelect.Toggle = key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "toggle"),
	)

	field := &interactiveField{
		MultiSelect: multiSelect,
		labels:      make(map[SelectionID]string, len(items)),
		groups:      make(map[SelectionID]string, len(items)),
		disabled:    make(map[SelectionID]bool, len(items)),
		toggle:      keys.MultiSelect.Toggle,
		bulkToggle:  keys.MultiSelect.SelectAll,
		theme:       s.theme,
	}
	for _, item := range items {
		id := itemID(item)
		field.labels[id] = item.Label
		if field.labels[id] == "" {
			field.labels[id] = itemName(item)
		}
		field.groups[id] = item.Group
		field.disabled[id] = item.Disabled
		field.hasDisabled = field.hasDisabled || item.Disabled
	}

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

	selectedSet := make(map[SelectionID]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	ordered := make([]SelectionID, 0, len(selectedSet))
	for _, item := range items {
		if item.Disabled {
			continue
		}
		id := itemID(item)
		if _, ok := selectedSet[id]; ok {
			ordered = append(ordered, id)
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
	*huh.MultiSelect[SelectionID]
	labels      map[SelectionID]string
	groups      map[SelectionID]string
	disabled    map[SelectionID]bool
	toggle      key.Binding
	bulkToggle  key.Binding
	theme       Theme
	hasDisabled bool
}

func (f *interactiveField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(keyMsg, f.bulkToggle) && f.hasDisabled {
			return f, nil
		}
		if key.Matches(keyMsg, f.toggle) {
			if hovered, found := f.MultiSelect.Hovered(); found && f.disabled[hovered] {
				return f, nil
			}
		}
	}
	updated, command := f.MultiSelect.Update(msg)
	f.MultiSelect = updated.(*huh.MultiSelect[SelectionID])
	return f, command
}

func (f *interactiveField) View() string {
	view := grayDisabledRows(f.MultiSelect.View(), f.labels, f.disabled, f.theme)
	view = groupSelectionRowsWithCreators(view, f.labels, f.groups, f.disabled, f.theme)
	hovered, ok := f.MultiSelect.Hovered()
	if !ok || f.disabled[hovered] {
		return view
	}
	return boldActiveLabel(view, f.labels[hovered])
}

func groupSelectionRows(view string, labels map[SelectionID]string, disabled map[SelectionID]bool, theme Theme) string {
	return groupSelectionRowsWithCreators(view, labels, nil, disabled, theme)
}

func groupSelectionRowsWithCreators(view string, labels map[SelectionID]string, groups map[SelectionID]string, disabled map[SelectionID]bool, theme Theme) string {
	lines := strings.Split(view, "\n")
	grouped := make([]string, 0, len(lines)+3)
	section := ""
	creator := ""
	for _, line := range lines {
		id, isRow, isDisabled := selectionRow(ansi.Strip(line), labels, disabled)
		if isRow {
			currentSection := "available"
			if isDisabled {
				currentSection = "installed"
			}
			if currentSection != section {
				if len(grouped) > 0 {
					grouped = append(grouped, "")
				}
				if isDisabled {
					grouped = append(grouped, theme.Muted("Already installed"))
				} else {
					grouped = append(grouped, theme.Accent("Available to install"))
				}
				section = currentSection
				creator = ""
			}
			currentCreator := groups[id]
			if currentCreator != "" && currentCreator != creator {
				if creator != "" {
					grouped = append(grouped, "")
				}
				grouped = append(grouped, theme.Accent(currentCreator))
				creator = currentCreator
			} else if currentCreator == "" {
				creator = ""
			}
		}
		grouped = append(grouped, line)
	}
	return strings.Join(grouped, "\n")
}

func selectionRow(line string, labels map[SelectionID]string, disabled map[SelectionID]bool) (SelectionID, bool, bool) {
	for id, label := range labels {
		if strings.Contains(line, label) {
			return id, true, disabled[id]
		}
	}
	return "", false, false
}

func grayDisabledRows(view string, labels map[SelectionID]string, disabled map[SelectionID]bool, theme Theme) string {
	lines := strings.Split(view, "\n")
	for index, line := range lines {
		plain := ansi.Strip(line)
		for id, isDisabled := range disabled {
			if !isDisabled || !strings.Contains(plain, labels[id]) {
				continue
			}
			plain = strings.Replace(plain, "[✗]", "[-]", 1)
			lines[index] = theme.Muted(plain)
			break
		}
	}
	return strings.Join(lines, "\n")
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

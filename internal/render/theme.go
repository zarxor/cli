package render

import (
	"image/color"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

type ColorMode uint8

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

type ThemeOptions struct {
	Mode ColorMode
	Dark bool
	Env  []string
}

type Theme struct {
	enabled   bool
	dark      bool
	accent    lipgloss.Style
	success   lipgloss.Style
	danger    lipgloss.Style
	warning   lipgloss.Style
	muted     lipgloss.Style
	important lipgloss.Style
}

func NewTheme(opts ThemeOptions) Theme {
	enabled := opts.Mode != ColorNever && !hasNoColor(opts.Env)
	choose := lipgloss.LightDark(opts.Dark)
	return Theme{
		enabled:   enabled,
		dark:      opts.Dark,
		accent:    colorStyle(enabled, choose(lipgloss.Color("#007C91"), lipgloss.Color("#5FD7FF"))),
		success:   colorStyle(enabled, choose(lipgloss.Color("#168A45"), lipgloss.Color("#5FD787"))),
		danger:    colorStyle(enabled, choose(lipgloss.Color("#C72C41"), lipgloss.Color("#FF5F6D"))),
		warning:   colorStyle(enabled, choose(lipgloss.Color("#9A6700"), lipgloss.Color("#FFD75F"))),
		muted:     colorStyle(enabled, choose(lipgloss.Color("#667085"), lipgloss.Color("#8A8A8A"))),
		important: lipgloss.NewStyle().Bold(enabled),
	}
}

func AutoTheme(in io.Reader, out io.Writer, env []string) Theme {
	inputTTY, outputTTY := streamsAreTerminal(in, out)
	dark := true
	if inputTTY && outputTTY {
		inputFile := in.(*os.File)
		outputFile := out.(*os.File)
		dark = lipgloss.HasDarkBackground(inputFile, outputFile)
	}
	return autoTheme(inputTTY, outputTTY, dark, env)
}

func autoTheme(inputTTY, outputTTY, dark bool, env []string) Theme {
	mode := ColorNever
	if inputTTY && outputTTY {
		mode = ColorAlways
	}
	return NewTheme(ThemeOptions{Mode: mode, Dark: dark, Env: env})
}

func streamsAreTerminal(in io.Reader, out io.Writer) (bool, bool) {
	inputFile, inputOK := in.(*os.File)
	outputFile, outputOK := out.(*os.File)
	inputTTY := inputOK && term.IsTerminal(int(inputFile.Fd()))
	outputTTY := outputOK && term.IsTerminal(int(outputFile.Fd()))
	return inputTTY, outputTTY
}

func hasNoColor(env []string) bool {
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == "NO_COLOR" {
			return true
		}
	}
	return false
}

func colorStyle(enabled bool, color color.Color) lipgloss.Style {
	style := lipgloss.NewStyle()
	if enabled {
		style = style.Foreground(color)
	}
	return style
}

func (t Theme) Accent(value string) string {
	return t.accent.Render(value)
}

func (t Theme) Success(value string) string {
	return t.success.Render(value)
}

func (t Theme) Danger(value string) string {
	return t.danger.Render(value)
}

func (t Theme) Warning(value string) string {
	return t.warning.Render(value)
}

func (t Theme) Muted(value string) string {
	return t.muted.Render(value)
}

func (t Theme) Important(value string) string {
	return t.important.Render(value)
}

func (t Theme) SelectedMarker() string {
	return "[" + t.Success("✓") + "]"
}

func (t Theme) UnselectedMarker() string {
	return "[" + t.Danger("✗") + "]"
}

func (t Theme) HuhTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		theme := NewTheme(ThemeOptions{
			Mode: map[bool]ColorMode{true: ColorAlways, false: ColorNever}[t.enabled],
			Dark: isDark,
		})
		styles := huh.ThemeBase(isDark)
		styles.Focused.Title = theme.accent.Bold(t.enabled)
		styles.Focused.Description = theme.muted
		styles.Focused.ErrorIndicator = theme.danger.SetString(" ✗")
		styles.Focused.ErrorMessage = theme.danger
		styles.Focused.MultiSelectSelector = lipgloss.NewStyle().SetString(theme.Accent("❯") + " ")
		styles.Focused.SelectedPrefix = lipgloss.NewStyle().SetString(theme.SelectedMarker() + " ")
		styles.Focused.UnselectedPrefix = lipgloss.NewStyle().SetString(theme.UnselectedMarker() + " ")
		styles.Blurred = styles.Focused
		styles.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
		styles.Group.Title = styles.Focused.Title
		styles.Group.Description = styles.Focused.Description
		styles.Help = huh.ThemeBase(isDark).Help
		return styles
	})
}

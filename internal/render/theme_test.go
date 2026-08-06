package render

import (
	"strings"
	"testing"
)

func TestThemeSelectionMarkersColorOnlyTheSymbol(t *testing.T) {
	theme := NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true})

	for _, test := range []struct {
		name   string
		marker string
		plain  string
	}{
		{name: "selected", marker: theme.SelectedMarker(), plain: "[✓]"},
		{name: "unselected", marker: theme.UnselectedMarker(), plain: "[✗]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := stripANSI(test.marker); got != test.plain {
				t.Fatalf("marker without ANSI = %q, want %q", got, test.plain)
			}
			if !strings.HasPrefix(test.marker, "[") || !strings.HasSuffix(test.marker, "]") {
				t.Fatalf("marker = %q, want neutral brackets outside ANSI styling", test.marker)
			}
			if start, reset := strings.Index(test.marker, "\x1b["), strings.LastIndex(test.marker, "\x1b[0m"); start <= 0 || reset >= len(test.marker)-1 {
				t.Fatalf("marker = %q, want ANSI styling confined inside brackets", test.marker)
			}
		})
	}
}

func TestThemeNeverColorReturnsPlainSemanticText(t *testing.T) {
	theme := NewTheme(ThemeOptions{Mode: ColorNever, Dark: true})

	for name, got := range map[string]string{
		"accent":    theme.Accent("heading"),
		"success":   theme.Success("installed"),
		"danger":    theme.Danger("failed"),
		"warning":   theme.Warning("dry-run"),
		"muted":     theme.Muted("hint"),
		"important": theme.Important("Git"),
	} {
		if strings.Contains(got, "\x1b[") {
			t.Fatalf("%s output = %q, want no ANSI", name, got)
		}
	}
}

func TestAutoThemeRequiresBothTerminalStreams(t *testing.T) {
	for _, test := range []struct {
		name      string
		inputTTY  bool
		outputTTY bool
		wantColor bool
	}{
		{name: "both terminals", inputTTY: true, outputTTY: true, wantColor: true},
		{name: "redirected input", inputTTY: false, outputTTY: true},
		{name: "redirected output", inputTTY: true, outputTTY: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			theme := autoTheme(test.inputTTY, test.outputTTY, true, nil)
			gotColor := strings.Contains(theme.Success("installed"), "\x1b[")
			if gotColor != test.wantColor {
				t.Fatalf("color enabled = %v, want %v", gotColor, test.wantColor)
			}
		})
	}
}

func TestAutoThemeHonorsNoColorRegardlessOfValue(t *testing.T) {
	for _, value := range []string{"NO_COLOR=", "NO_COLOR=1"} {
		theme := autoTheme(true, true, true, []string{value})
		if got := theme.Success("installed"); strings.Contains(got, "\x1b[") {
			t.Fatalf("%s output = %q, want no ANSI", value, got)
		}
	}
}

func TestHuhThemeUsesApprovedSelectionSymbols(t *testing.T) {
	theme := NewTheme(ThemeOptions{Mode: ColorNever, Dark: true}).HuhTheme().Theme(true)

	if got := theme.Focused.MultiSelectSelector.String(); got != "❯ " {
		t.Fatalf("selector = %q, want %q", got, "❯ ")
	}
	if got := theme.Focused.SelectedPrefix.String(); got != "[✓] " {
		t.Fatalf("selected prefix = %q, want %q", got, "[✓] ")
	}
	if got := theme.Focused.UnselectedPrefix.String(); got != "[✗] " {
		t.Fatalf("unselected prefix = %q, want %q", got, "[✗] ")
	}
}

func TestHuhThemeStylesTitleAsBoldAccent(t *testing.T) {
	theme := NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true}).HuhTheme().Theme(true)

	if !theme.Focused.Title.GetBold() {
		t.Fatal("focused title is not bold")
	}
	r, g, b, _ := theme.Focused.Title.GetForeground().RGBA()
	if r != 0x5f*0x101 || g != 0xd7*0x101 || b != 0xff*0x101 {
		t.Fatalf("focused title foreground = %#04x %#04x %#04x, want dark accent", r, g, b)
	}
}

func stripANSI(value string) string {
	for {
		start := strings.Index(value, "\x1b[")
		if start < 0 {
			return value
		}
		end := start + 2
		for end < len(value) && (value[end] < '@' || value[end] > '~') {
			end++
		}
		if end < len(value) {
			end++
		}
		value = value[:start] + value[end:]
	}
}

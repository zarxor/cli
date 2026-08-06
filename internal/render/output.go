package render

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Renderer writes semantic, human-facing CLI output.
type Renderer struct {
	writer         io.Writer
	theme          Theme
	interactive    bool
	progressActive bool
}

type ResultRow struct {
	Action string
	Tool   string
	Status string
	Err    error
}

func NewRenderer(writer io.Writer, theme Theme) *Renderer {
	if writer == nil {
		writer = io.Discard
	}
	interactive := false
	if file, ok := writer.(*os.File); ok {
		interactive = term.IsTerminal(int(file.Fd()))
	}
	return &Renderer{writer: writer, theme: theme, interactive: interactive}
}

func NewPlainRenderer(writer io.Writer) *Renderer {
	return NewRenderer(writer, NewTheme(ThemeOptions{Mode: ColorNever}))
}

func (r *Renderer) Result(row ResultRow) error {
	var line string
	switch row.Status {
	case "installed", "updated", "up-to-date":
		line = r.theme.Success("✓") + " " + r.theme.Success(row.Status) + " " + r.theme.Important(row.Tool)
	case "dry-run":
		line = r.theme.Warning("!") + " " + r.theme.Warning(row.Action) + " " + r.theme.Important(row.Tool) + ": " + r.theme.Warning("dry-run")
	case "skipped":
		line = r.theme.Muted("- skipped " + row.Tool)
	case "failed":
		line = r.theme.Danger("✗") + " " + r.theme.Danger(row.Action+" "+row.Tool+" failed")
	default:
		return fmt.Errorf("unsupported result status %q", row.Status)
	}
	if row.Err != nil && (row.Status == "failed" || row.Status == "skipped") {
		line += ": " + row.Err.Error()
	}
	return r.line(line)
}

func (r *Renderer) Cancelled() error {
	return r.line(r.theme.Muted("Cancelled — no changes made"))
}

func (r *Renderer) Version(name, version string) error {
	return r.line(r.theme.Important(name) + " " + r.theme.Accent(version))
}

func (r *Renderer) Error(err error) error {
	return r.line(r.theme.Danger("✗") + " " + r.theme.Danger("error: "+err.Error()))
}

// Progress reports a long-running stage without implying that the operation
// has completed. It intentionally uses a full line so redirected output stays
// readable and terminals do not need cursor-control support.
func (r *Renderer) Progress(message string) error {
	return r.line(r.theme.Muted("• " + message))
}

// ProgressBar redraws one progress line and finishes it with a newline when
// the operation reaches total. Carriage returns keep redirected output
// compact, while terminals also receive a clear-line escape before redraws.
func (r *Renderer) ProgressBar(label string, current, total int) error {
	if current < 0 || total < 0 || current > total {
		return fmt.Errorf("invalid progress %d/%d", current, total)
	}
	const width = 20
	filled := 0
	if total > 0 {
		filled = current * width / total
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	message := fmt.Sprintf("• %s [%s] %d/%d", label, bar, current, total)
	prefix := "\r"
	if r.interactive {
		prefix += "\x1b[2K"
	}
	if _, err := fmt.Fprint(r.writer, prefix+r.theme.Muted(message)); err != nil {
		return err
	}
	r.progressActive = true
	if current == total {
		return r.FinishProgress()
	}
	return nil
}

// FinishProgress terminates an active progress line before subsequent output.
func (r *Renderer) FinishProgress() error {
	if !r.progressActive {
		return nil
	}
	r.progressActive = false
	_, err := fmt.Fprintln(r.writer)
	return err
}

// Help applies restrained semantic styling to already-rendered Cobra help.
func (r *Renderer) Help(help string) error {
	lines := strings.Split(strings.TrimSuffix(help, "\n"), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if line == trimmed && strings.HasSuffix(trimmed, ":") {
			lines[i] = strings.Replace(line, trimmed, r.theme.Accent(trimmed), 1)
			continue
		}
		if strings.HasPrefix(line, "  ") && trimmed != "" {
			fields := strings.Fields(trimmed)
			if len(fields) > 0 && (strings.HasPrefix(fields[0], "-") || !strings.Contains(fields[0], ":")) {
				lines[i] = strings.Replace(line, fields[0], r.theme.Important(fields[0]), 1)
			}
		}
	}
	return r.line(strings.Join(lines, "\n"))
}

func (r *Renderer) line(value string) error {
	if err := r.FinishProgress(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(r.writer, value)
	return err
}

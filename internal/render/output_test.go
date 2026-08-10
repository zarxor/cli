package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/zarxor/cli/internal/tools"
)

func TestRendererResultUsesSemanticSymbolsAndWords(t *testing.T) {
	tests := []struct{ status, want string }{
		{"installed", "✓ installed Git"},
		{"updated", "✓ updated Git"},
		{"up-to-date", "✓ up-to-date Git"},
		{"dry-run", "! install Git: dry-run"},
		{"skipped", "- skipped Git"},
		{"failed", "✗ install Git failed"},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			var output bytes.Buffer
			r := NewPlainRenderer(&output)
			if err := r.Result(ResultRow{Action: "install", Tool: "Git", Status: test.status}); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(output.String()); got != test.want {
				t.Fatalf("Result() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRendererForcedColorPreservesPlainText(t *testing.T) {
	var output bytes.Buffer
	r := NewRenderer(&output, NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true}))
	rows := []ResultRow{
		{Action: "install", Tool: "Git", Status: "installed"},
		{Action: "install", Tool: "Docker", Status: "dry-run"},
		{Action: "install", Tool: "Bun", Status: "skipped", Err: errors.New("dependency failed")},
		{Action: "install", Tool: "GitHub CLI", Status: "failed", Err: errors.New("boom")},
	}
	for _, row := range rows {
		if err := r.Result(row); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("forced output has no ANSI: %q", output.String())
	}
	want := "✓ installed Git\n! install Docker: dry-run\n- skipped Bun: dependency failed\n✗ install GitHub CLI failed: boom\n"
	if got := stripANSI(output.String()); got != want {
		t.Fatalf("stripped output = %q, want %q", got, want)
	}
}

func TestRendererOtherPublicOutput(t *testing.T) {
	var output bytes.Buffer
	r := NewPlainRenderer(&output)
	if err := r.Cancelled(); err != nil {
		t.Fatal(err)
	}
	if err := r.Version("Johan Bostrom CLI", "dev"); err != nil {
		t.Fatal(err)
	}
	if err := r.Error(errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if err := r.Help("Usage:\n  jb tools\n"); err != nil {
		t.Fatal(err)
	}
	want := "Cancelled — no changes made\nJohan Bostrom CLI dev\n✗ error: boom\nUsage:\n  jb tools\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestRendererProgressBarRedrawsOneLine(t *testing.T) {
	var output bytes.Buffer
	r := NewPlainRenderer(&output)
	for current := 0; current <= 3; current++ {
		if err := r.ProgressBar("Checking installed tools", current, 3); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(output.String(), "\n"); got != 1 {
		t.Fatalf("progress newlines = %d, want one: %q", got, output.String())
	}
	if !strings.Contains(output.String(), "[####################] 3/3") {
		t.Fatalf("completed progress = %q, want a full progress bar", output.String())
	}
}

func TestRendererVersionTableForcedColorStripsToPlain(t *testing.T) {
	rows := []VersionRow{{Tool: tools.Tool{Name: "Git"}, CurrentVersion: "2.48.0", CandidateVersion: "2.49.0"}}
	var plain, colored bytes.Buffer
	if err := NewPlainRenderer(&plain).VersionTable(rows); err != nil {
		t.Fatal(err)
	}
	if err := NewRenderer(&colored, NewTheme(ThemeOptions{Mode: ColorAlways, Dark: true})).VersionTable(rows); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatalf("colored table has no ANSI: %q", colored.String())
	}
	if got := stripANSI(colored.String()); got != plain.String() {
		t.Fatalf("stripped = %q, plain = %q", got, plain.String())
	}
}

func TestRendererPropagatesWriterFailures(t *testing.T) {
	want := errors.New("write failed")
	r := NewPlainRenderer(errorWriter{want})
	checks := []func() error{
		func() error { return r.Result(ResultRow{Action: "install", Tool: "Git", Status: "installed"}) },
		r.Cancelled,
		func() error { return r.Version("jb", "dev") },
		func() error { return r.Error(errors.New("boom")) },
		func() error { return r.Help("Usage:\n") },
		func() error { return r.VersionTable(nil) },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, want) {
			t.Fatalf("check %d error = %v, want %v", i, err, want)
		}
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

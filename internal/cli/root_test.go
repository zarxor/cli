package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zarxor/scripts/internal/render"
)

func TestExecuteVersion(t *testing.T) {
	output := new(bytes.Buffer)
	err := ExecuteWithIO(context.Background(), []string{"version"}, output, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Johan Bostrom CLI") {
		t.Fatal(output.String())
	}
}

func TestExecuteVersionWritesTrailingNewline(t *testing.T) {
	output := new(bytes.Buffer)
	err := ExecuteWithIO(context.Background(), []string{"version"}, output, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "Johan Bostrom CLI dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestExecuteHelpListsToolsAndVersion(t *testing.T) {
	output := new(bytes.Buffer)
	err := ExecuteWithIO(context.Background(), []string{"help"}, output, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "tools") {
		t.Fatal(output.String())
	}
	if !strings.Contains(output.String(), "version") {
		t.Fatal(output.String())
	}
	if !strings.Contains(output.String(), "completion") {
		t.Fatal(output.String())
	}
}

func TestForcedColorVersionAndHelpPreserveText(t *testing.T) {
	forced := func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorAlways, Dark: true})
	}
	for _, args := range [][]string{{"version"}, {"help"}} {
		var output bytes.Buffer
		root := newRootCommandWithTheme(&recordingToolsService{}, forced)
		root.SetArgs(args)
		root.SetIn(strings.NewReader("\n"))
		root.SetOut(&output)
		root.SetErr(io.Discard)
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "\x1b[") {
			t.Fatalf("%v output has no ANSI: %q", args, output.String())
		}
		if args[0] == "version" && stripCLIANSI(output.String()) != "Johan Bostrom CLI dev\n" {
			t.Fatalf("version output = %q", output.String())
		}
	}
}

func TestRedirectedHelpHasNoANSI(t *testing.T) {
	var output bytes.Buffer
	if err := ExecuteWithIO(context.Background(), []string{"help"}, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("help contains ANSI: %q", output.String())
	}
}

func TestCompletionRemainsPlainWithForcedHumanTheme(t *testing.T) {
	var output bytes.Buffer
	root := newRootCommandWithTheme(&recordingToolsService{}, func(*cobra.Command) render.Theme {
		return render.NewTheme(render.ThemeOptions{Mode: render.ColorAlways, Dark: true})
	})
	root.SetArgs([]string{"completion", "powershell"})
	root.SetOut(&output)
	root.SetErr(io.Discard)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatal("completion output contains ANSI")
	}
}

func TestPrintErrorUsesSemanticPrefixAndPreservesWriterFailure(t *testing.T) {
	var output bytes.Buffer
	if err := printErrorWithTheme(&output, errors.New("boom"), render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "✗ error: boom\n"; got != want {
		t.Fatalf("PrintError output = %q, want %q", got, want)
	}
	wantErr := errors.New("writer failed")
	if err := printErrorWithTheme(cliErrorWriter{wantErr}, errors.New("boom"), render.NewTheme(render.ThemeOptions{Mode: render.ColorNever})); !errors.Is(err, wantErr) {
		t.Fatalf("PrintError error = %v, want %v", err, wantErr)
	}
}

var cliANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripCLIANSI(value string) string { return cliANSI.ReplaceAllString(value, "") }

type cliErrorWriter struct{ err error }

func (w cliErrorWriter) Write([]byte) (int, error) { return 0, w.err }

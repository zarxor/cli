package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
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

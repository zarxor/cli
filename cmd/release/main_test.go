package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunExecutesReleaseThroughRealProcessBoundary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the checked-in helper integration currently exercises the Windows shell path")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	helper := filepath.Join(binDir, "fake-tool.exe")
	build := exec.Command(realGo, "build", "-o", helper, "./cmd/release/testdata/fake-tool")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake tool: %v\n%s", err, output)
	}
	for _, name := range []string{"git.exe", "go.exe", "gh.exe", "pwsh.exe"} {
		contents, err := os.ReadFile(helper)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, name), contents, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	fakeRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fakeRoot, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "commands.log")
	t.Setenv("PATH", binDir)
	t.Setenv("JB_FAKE_ROOT", fakeRoot)
	t.Setenv("JB_FAKE_LOG", logPath)

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	code := run(context.Background(), fakeRoot, "windows", strings.NewReader("1\nyes\n"), out, errOut)
	if code != 0 {
		t.Fatalf("run() code = %d\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	logContents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logContents)
	for _, call := range []string{
		"git tag -a v1.2.4 abc123 -m Release v1.2.4",
		"git push origin refs/tags/v1.2.4",
		"gh release create v1.2.4",
		"gh release edit v1.2.4 --draft=false",
		"gh release view v1.2.4 --json isDraft,url,assets",
	} {
		if !strings.Contains(log, call) {
			t.Errorf("command log missing %q:\n%s", call, log)
		}
	}
	if !strings.Contains(out.String(), "Published v1.2.4") {
		t.Fatalf("published output missing:\n%s", out)
	}
}

func TestRunTreatsCancellationAsSuccess(t *testing.T) {
	code := exitCodeForError(context.Canceled)
	if code != 1 {
		t.Fatalf("ordinary error exit code = %d, want 1", code)
	}
	code = exitCodeForError(errReleaseCancelled)
	if code != 0 {
		t.Fatalf("cancellation exit code = %d, want 0", code)
	}
}

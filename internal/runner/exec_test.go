package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestExecRunIncludesStderrOnFailure(t *testing.T) {
	t.Setenv("JB_RUNNER_HELPER", "1")

	result, err := (Exec{}).Run(context.Background(), os.Args[0], "-test.run=TestExecRunHelper")
	if err == nil {
		t.Fatal("Run() error = nil, want command failure")
	}
	if result.ExitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(err.Error(), "pnpm: synthetic detection failure") {
		t.Fatalf("Run() error = %v, want captured stderr", err)
	}
}

func TestExecRunHelper(t *testing.T) {
	if os.Getenv("JB_RUNNER_HELPER") != "1" {
		return
	}
	fmt.Fprintln(os.Stderr, "pnpm: synthetic detection failure")
	os.Exit(1)
}

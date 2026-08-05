package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Exec executes commands directly, without involving a shell.
type Exec struct{}

func NewExec() Runner { return Exec{} }

func (Exec) LookPath(_ context.Context, name string) (string, error) {
	return exec.LookPath(name)
}

func (Exec) Run(ctx context.Context, command string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}

	result.ExitCode = -1
	if exitError, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitError.ExitCode()
	}
	return result, fmt.Errorf("command %q failed with exit code %d: %w", command, result.ExitCode, err)
}

// Package runner provides structured external-command execution boundaries.
package runner

import "context"

type Runner interface {
	LookPath(ctx context.Context, name string) (string, error)
	Run(ctx context.Context, command string, args ...string) (Result, error)
}

type Elevation interface {
	RunElevated(ctx context.Context, command string, args ...string) (Result, error)
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Command is an immutable-style record of a structured command invocation.
type Command struct {
	Command string
	Args    []string
}

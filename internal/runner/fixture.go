package runner

import (
	"context"
	"fmt"
	"sync"
)

// Fixture is a deterministic Runner and Elevation implementation for tests.
// Commands are recorded as argument slices and never executed on the host.
type Fixture struct {
	mu             sync.Mutex
	Commands       []Command
	LookPaths      map[string]string
	LookPathErrors map[string]error
	responses      map[string]response
}

type response struct {
	result Result
	err    error
}

func NewFixture() *Fixture {
	return &Fixture{
		LookPaths:      make(map[string]string),
		LookPathErrors: make(map[string]error),
		responses:      make(map[string]response),
	}
}

func (f *Fixture) Set(command string, args []string, result Result, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[commandKey(command, args)] = response{result: result, err: err}
}

func (f *Fixture) LookPath(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.LookPathErrors[name]; ok {
		return "", err
	}
	path, ok := f.LookPaths[name]
	if !ok {
		return "", fmt.Errorf("executable %q not found", name)
	}
	return path, nil
}

func (f *Fixture) Run(_ context.Context, command string, args ...string) (Result, error) {
	commandArgs := append([]string(nil), args...)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Commands = append(f.Commands, Command{Command: command, Args: commandArgs})
	if configured, ok := f.responses[commandKey(command, args)]; ok {
		return configured.result, configured.err
	}
	return Result{}, nil
}

func (f *Fixture) RunElevated(ctx context.Context, command string, args ...string) (Result, error) {
	return f.Run(ctx, command, args...)
}

func commandKey(command string, args []string) string {
	key := command
	for _, arg := range args {
		key += "\x00" + arg
	}
	return key
}

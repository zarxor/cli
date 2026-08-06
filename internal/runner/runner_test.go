package runner

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFixturePreservesExactArguments(t *testing.T) {
	fixture := NewFixture()
	args := []string{"install", "package name", "--literal=$HOME"}
	fixture.Set("pkg", args, Result{Stdout: "installed"}, nil)

	result, err := fixture.Run(context.Background(), "pkg", args...)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "installed" {
		t.Fatalf("stdout = %q, want installed", result.Stdout)
	}
	if len(fixture.Commands) != 1 {
		t.Fatalf("recorded commands = %d, want 1", len(fixture.Commands))
	}
	got := fixture.Commands[0]
	if got.Command != "pkg" || !reflect.DeepEqual(got.Args, args) {
		t.Fatalf("recorded command = %#v, want pkg %#v", got, args)
	}
}

func TestFixturePreservesConfiguredExitCode(t *testing.T) {
	fixture := NewFixture()
	wantErr := errors.New("installer failed")
	fixture.Set("installer", []string{"--run"}, Result{Stderr: "bad", ExitCode: 23}, wantErr)

	result, err := fixture.Run(context.Background(), "installer", "--run")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result.ExitCode != 23 {
		t.Fatalf("exit code = %d, want 23", result.ExitCode)
	}
}

package service

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zarxor/cli/internal/runner"
)

func TestT3CodeInstallUsesNVMAsTheInvokingUser(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	fixture.LookPaths[nvmExec] = nvmExec
	baseDir := filepath.Join(t.TempDir(), "t3 data")
	wantArgs := []string{
		"HOME=" + home,
		"XDG_RUNTIME_DIR=/run/user/1000",
		"NVM_DIR=" + filepath.Join(home, ".nvm"),
		"NODE_VERSION=lts/*",
		nvmExec,
		"npx", "--yes", "t3@latest", "service", "install", "--base-dir", baseDir,
	}
	fixture.Set("env", wantArgs, runner.Result{Stdout: "Installed T3 Code service"}, nil)
	manager := NewT3CodeManager(fixture, Config{
		Platform:    "linux",
		Home:        home,
		InvokingUID: 1000,
	})

	result, err := manager.Run(context.Background(), Install, baseDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "Installed T3 Code service" {
		t.Fatalf("output = %q, want service output", result.Output)
	}
	if len(fixture.Commands) != 1 || fixture.Commands[0].Command != "env" || !reflect.DeepEqual(fixture.Commands[0].Args, wantArgs) {
		t.Fatalf("commands = %#v, want env %#v", fixture.Commands, wantArgs)
	}
}

func TestT3CodeFallsBackToNpxOutsideNvm(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	fixture.LookPaths["npx"] = "/usr/bin/npx"
	wantArgs := []string{"HOME=" + home, "/usr/bin/npx", "--yes", "t3@latest", "service", "update"}
	fixture.Set("env", wantArgs, runner.Result{}, nil)
	manager := NewT3CodeManager(fixture, Config{Platform: "linux", Home: home})

	if _, err := manager.Run(context.Background(), Update, "", false); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 || fixture.Commands[0].Command != "env" || !reflect.DeepEqual(fixture.Commands[0].Args, wantArgs) {
		t.Fatalf("commands = %#v, want env %#v", fixture.Commands, wantArgs)
	}
}

func TestT3CodeRunsAsInvokingUserWhenJbIsRoot(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	nvmExec := filepath.Join(home, ".nvm", "nvm-exec")
	fixture.LookPaths[nvmExec] = nvmExec
	wantArgs := []string{
		"-H", "-u", "johan", "env",
		"HOME=" + home,
		"XDG_RUNTIME_DIR=/run/user/1000",
		"NVM_DIR=" + filepath.Join(home, ".nvm"),
		"NODE_VERSION=lts/*",
		nvmExec,
		"npx", "--yes", "t3@latest", "service", "install",
	}
	fixture.Set("sudo", wantArgs, runner.Result{}, nil)
	manager := NewT3CodeManager(fixture, Config{
		Platform:     "linux",
		Home:         home,
		Root:         true,
		InvokingUser: "johan",
		InvokingUID:  1000,
	})

	if _, err := manager.Run(context.Background(), Install, "", false); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 || fixture.Commands[0].Command != "sudo" || !reflect.DeepEqual(fixture.Commands[0].Args, wantArgs) {
		t.Fatalf("commands = %#v, want sudo %#v", fixture.Commands, wantArgs)
	}
}

func TestT3CodeDryRunDoesNotInvokeTheServiceCli(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	fixture.LookPaths["npx"] = "/usr/bin/npx"
	manager := NewT3CodeManager(fixture, Config{Platform: "linux", Home: home})

	result, err := manager.Run(context.Background(), Install, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || !strings.Contains(result.Command, "t3@latest service install") {
		t.Fatalf("dry-run result = %#v, want an install command", result)
	}
	if len(fixture.Commands) != 0 {
		t.Fatalf("commands = %#v, want no executed command", fixture.Commands)
	}
}

func TestT3CodeRejectsUnsupportedPlatformBeforeCommandLookup(t *testing.T) {
	fixture := runner.NewFixture()
	manager := NewT3CodeManager(fixture, Config{Platform: "windows", Home: `C:\Users\johan`})

	_, err := manager.Run(context.Background(), Install, "", false)
	if err == nil || !strings.Contains(err.Error(), "requires Linux with systemd") {
		t.Fatalf("error = %v, want Linux/systemd error", err)
	}
	if len(fixture.Commands) != 0 {
		t.Fatalf("commands = %#v, want no command", fixture.Commands)
	}
}

func TestT3CodeRejectsMissingNodeRuntime(t *testing.T) {
	fixture := runner.NewFixture()
	manager := NewT3CodeManager(fixture, Config{Platform: "linux", Home: t.TempDir()})

	_, err := manager.Run(context.Background(), Install, "", false)
	if err == nil || !strings.Contains(err.Error(), "find npx or nvm") {
		t.Fatalf("error = %v, want missing runtime error", err)
	}
}

func TestT3CodeCanRestartServiceThroughSystemd(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	fixture.LookPaths["systemctl"] = "/usr/bin/systemctl"
	fixture.Set("env", []string{"HOME=" + home, "/usr/bin/systemctl", "--user", "restart", t3CodeServiceUnit}, runner.Result{Stdout: "restarted\n"}, nil)
	manager := NewT3CodeManager(fixture, Config{Platform: "linux", Home: home})

	result, err := manager.Run(context.Background(), Restart, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "restarted" || len(fixture.Commands) != 1 || fixture.Commands[0].Command != "env" {
		t.Fatalf("result = %#v, commands = %#v", result, fixture.Commands)
	}
}

func TestT3CodeLogsUseUserJournalWithBoundedLines(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	fixture.LookPaths["journalctl"] = "/usr/bin/journalctl"
	fixture.Set("env", []string{"HOME=" + home, "/usr/bin/journalctl", "--user-unit", t3CodeServiceUnit, "--no-pager", "-n", "100", "--follow"}, runner.Result{Stdout: "logs"}, nil)
	manager := NewT3CodeManager(fixture, Config{Platform: "linux", Home: home})

	if _, err := manager.RunWithOptions(context.Background(), Logs, "", false, RunOptions{Lines: 0, Follow: true}); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 || fixture.Commands[0].Command != "env" {
		t.Fatalf("commands = %#v", fixture.Commands)
	}
}

func TestT3CodeRepairDelegatesToOfficialUpdate(t *testing.T) {
	fixture := runner.NewFixture()
	home := t.TempDir()
	fixture.LookPaths["npx"] = "/usr/bin/npx"
	want := []string{"HOME=" + home, "/usr/bin/npx", "--yes", "t3@latest", "service", "update"}
	fixture.Set("env", want, runner.Result{}, nil)
	manager := NewT3CodeManager(fixture, Config{Platform: "linux", Home: home})
	if _, err := manager.Run(context.Background(), Repair, "", false); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 || !reflect.DeepEqual(fixture.Commands[0].Args, want) {
		t.Fatalf("commands = %#v, want %v", fixture.Commands, want)
	}
}

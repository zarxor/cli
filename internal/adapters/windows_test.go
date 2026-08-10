package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zarxor/cli/internal/detect"
	"github.com/zarxor/cli/internal/plan"
	"github.com/zarxor/cli/internal/profile"
	"github.com/zarxor/cli/internal/runner"
)

func TestWindowsInstallsSystemToolsThroughWinGet(t *testing.T) {
	cases := []struct {
		id   profile.ToolID
		want []string
	}{
		{profile.Git, []string{"install", "--id", "Git.Git", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
		{profile.GitHubCLI, []string{"install", "--id", "GitHub.cli", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
		{profile.Docker, []string{"install", "--id", "Docker.DockerDesktop", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
		{profile.NVM, []string{"install", "--id", "CoreyButler.NVMforWindows", "--exact", "--accept-package-agreements", "--accept-source-agreements"}},
	}

	for _, test := range cases {
		t.Run(string(test.id), func(t *testing.T) {
			fixture := runner.NewFixture()
			elevation := &windowsFixtureElevation{fixture: fixture}
			adapter := NewWindowsAdapter(fixture, elevation)

			if err := adapter.Install(context.Background(), mustTool(t, test.id)); err != nil {
				t.Fatal(err)
			}
			if len(elevation.commands) != 1 {
				t.Fatalf("elevated commands = %#v, want one WinGet install", elevation.commands)
			}
			if got := elevation.commands[0]; got.Command != "winget" || !reflect.DeepEqual(got.Args, test.want) {
				t.Fatalf("WinGet command = %#v, want winget %#v", got, test.want)
			}
		})
	}
}

func TestWindowsDetectsMissingToolAndDoesNotUpdateIt(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.Set("winget", []string{"show", "--id", "Git.Git", "--exact"}, runner.Result{Stdout: "Version: 2.49.0\n"}, nil)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if got != (detect.Detection{Candidate: "2.49.0"}) {
		t.Fatalf("Detect() = %#v, want missing tool with candidate", got)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}
	for _, command := range fixture.Commands {
		if command.Command == "winget" && len(command.Args) > 0 && command.Args[0] == "upgrade" {
			t.Fatalf("Update() upgraded missing tool: %#v", fixture.Commands)
		}
	}
}

func TestWindowsTreatsMissingDockerComponentAsAbsent(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["docker"] = `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.Set(`C:\Program Files\Docker\Docker\resources\bin\docker.exe`, []string{"compose", "version"}, runner.Result{Stderr: "docker: 'compose' is not a docker command\n", ExitCode: 1}, errors.New("exit status 1"))
	fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	got, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerCompose))
	if err != nil {
		t.Fatal(err)
	}
	if got != (detect.Detection{Candidate: "4.42.0"}) {
		t.Fatalf("Detect() = %#v, want absent Compose with Docker Desktop candidate", got)
	}
}

func TestWindowsAlreadyCurrentDockerReportsDesktopPackageVersion(t *testing.T) {
	fixture := runner.NewFixture()
	dockerPath := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.LookPaths["docker"] = dockerPath
	fixture.Set(dockerPath, []string{"--version"}, runner.Result{Stdout: "Docker version 28.1.1\n"}, nil)
	fixture.Set(dockerPath, []string{"buildx", "version"}, runner.Result{Stdout: "github.com/docker/buildx v0.24.0\n"}, nil)
	fixture.Set(dockerPath, []string{"compose", "version"}, runner.Result{Stdout: "Docker Compose version v2.36.0\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	fixture.Set("winget", []string{"list", "--id", "Docker.DockerDesktop", "--exact", "--details"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	for _, id := range []profile.ToolID{profile.Docker, profile.DockerBuildx, profile.DockerCompose} {
		got, err := adapter.Detect(context.Background(), mustTool(t, id))
		if err != nil {
			t.Fatalf("Detect(%s): %v", id, err)
		}
		if got != (detect.Detection{Installed: true, Current: "4.42.0", Candidate: "4.42.0"}) {
			t.Fatalf("Detect(%s) = %#v, want installed Docker Desktop package at 4.42.0", id, got)
		}
	}
	listCount, showCount := 0, 0
	for _, command := range fixture.Commands {
		if command.Command != "winget" {
			continue
		}
		if len(command.Args) > 0 && command.Args[0] == "list" {
			listCount++
		}
		if len(command.Args) > 0 && command.Args[0] == "show" {
			showCount++
		}
	}
	if listCount != 1 || showCount != 1 {
		t.Fatalf("Docker Desktop package lookups = list %d/show %d, want one each", listCount, showCount)
	}
}

func TestWindowsPreservesGenuineDockerDetectionFailure(t *testing.T) {
	fixture := runner.NewFixture()
	path := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.LookPaths["docker"] = path
	wantErr := errors.New("access denied")
	fixture.Set(path, []string{"buildx", "version"}, runner.Result{Stderr: "access denied\n", ExitCode: 1}, wantErr)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	_, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerBuildx))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Detect() error = %v, want genuine failure", err)
	}
}

func TestWindowsBrokenPresentExecutablesRemainDetectionErrors(t *testing.T) {
	tests := []struct {
		name    string
		id      profile.ToolID
		result  runner.Result
		wantErr error
	}{
		{name: "dynamic loader failure", id: profile.NPM, result: runner.Result{Stderr: "npm: error while loading shared libraries: libnode.dll: No such file or directory\n", ExitCode: 127}, wantErr: errors.New("loader failed")},
		{name: "permission failure", id: profile.NPM, result: runner.Result{Stderr: "npm: Permission denied\n", ExitCode: 127}, wantErr: errors.New("permission denied")},
		{name: "arbitrary missing file", id: profile.NPM, result: runner.Result{Stderr: "npm: config: No such file or directory\n", ExitCode: 1}, wantErr: errors.New("config missing")},
		{name: "embedded module diagnostic", id: profile.NPM, result: runner.Result{Stderr: "module loader failure: npm: not found\n", ExitCode: 127}, wantErr: errors.New("module failed")},
		{name: "unrelated Docker unknown command", id: profile.DockerBuildx, result: runner.Result{Stderr: "docker daemon returned unknown command while loading plugin\n", ExitCode: 1}, wantErr: errors.New("plugin failed")},
		{name: "embedded Docker permission diagnostic", id: profile.DockerBuildx, result: runner.Result{Stderr: "permission denied: docker: 'buildx' is not a docker command\n", ExitCode: 1}, wantErr: errors.New("permission failed")},
		{name: "Docker diagnostic with success status", id: profile.DockerBuildx, result: runner.Result{Stderr: "docker: 'buildx' is not a docker command\n", ExitCode: 0}, wantErr: errors.New("transport failed")},
		{name: "Docker diagnostic with wrong nonzero status", id: profile.DockerBuildx, result: runner.Result{Stderr: "docker: 'buildx' is not a docker command\n", ExitCode: 127}, wantErr: errors.New("wrapper failed")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := runner.NewFixture()
			source := windowsSources[test.id]
			path := filepath.Join(t.TempDir(), source.executable+".exe")
			fixture.LookPaths[source.executable] = path
			fixture.Set(path, source.version, test.result, test.wantErr)
			adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

			_, err := adapter.Detect(context.Background(), mustTool(t, test.id))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Detect() error = %v, want broken-present error %v", err, test.wantErr)
			}
		})
	}
}

func TestWindowsDetectsCandidatesForInstalledUserTools(t *testing.T) {
	fixture := runner.NewFixture()
	paths := map[string]string{
		"nvm":  `C:\Users\johan\AppData\Roaming\nvm\nvm.exe`,
		"node": `C:\Program Files\nodejs\node.exe`, "npm": `C:\Program Files\nodejs\npm.cmd`,
		"corepack": `C:\Program Files\nodejs\corepack.cmd`, "pnpm": `C:\Program Files\nodejs\pnpm.cmd`,
		"yarn": `C:\Program Files\nodejs\yarn.cmd`, "codex": `C:\Program Files\nodejs\codex.cmd`,
		"bun": `C:\Program Files\nodejs\bun.exe`,
	}
	for executable, path := range paths {
		fixture.LookPaths[executable] = path
		fixture.Set(path, windowsSources[mustTool(t, profile.ToolID(executable)).ID].version, runner.Result{Stdout: "1.0.0\n"}, nil)
	}
	fixture.Set(paths["nvm"], []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS | OLD STABLE |\n| 26.1.0 | 24.5.0 | 22.1.0 |\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "CoreyButler.NVMforWindows", "--exact"}, runner.Result{Stdout: "Version: 1.2.2\n"}, nil)
	packages := map[profile.ToolID]string{
		profile.NPM: "npm", profile.Corepack: "corepack", profile.PNPM: "pnpm",
		profile.Yarn: "@yarnpkg/cli-dist", profile.Codex: "@openai/codex", profile.Bun: "bun",
	}
	for id, packageName := range packages {
		fixture.Set(paths["npm"], []string{"view", packageName, "version"}, runner.Result{Stdout: "2.0.0\n"}, nil)
		_ = id
	}
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture}, WindowsConfig{
		ProgramFiles: `C:\Program Files`, NVMHome: filepath.Dir(paths["nvm"]), NVMSymlink: filepath.Dir(paths["node"]),
	})
	cases := []struct {
		id   profile.ToolID
		want string
	}{
		{profile.NVM, "1.2.2"}, {profile.Node, "24.5.0"}, {profile.NPM, "2.0.0"}, {profile.Corepack, "2.0.0"},
		{profile.PNPM, "2.0.0"}, {profile.Yarn, "2.0.0"}, {profile.Codex, "2.0.0"}, {profile.Bun, "2.0.0"},
	}
	for _, test := range cases {
		t.Run(string(test.id), func(t *testing.T) {
			got, err := adapter.Detect(context.Background(), mustTool(t, test.id))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Installed || got.Candidate != test.want {
				t.Fatalf("Detect() = %#v, want candidate %q", got, test.want)
			}
		})
	}
}

func TestWindowsCandidateLookupFailuresPreserveSuccessfulDetection(t *testing.T) {
	tests := []struct {
		name       string
		id         profile.ToolID
		executable string
		current    string
		prepare    func(*runner.Fixture, string)
	}{
		{name: "nvm unavailable for Node candidate", id: profile.Node, executable: "node", current: "v22.1.0"},
		{
			name: "npm registry outage", id: profile.NPM, executable: "npm", current: "10.8.0",
			prepare: func(fixture *runner.Fixture, path string) {
				fixture.Set(path, []string{"view", "npm", "version"}, runner.Result{}, errors.New("registry unavailable"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := runner.NewFixture()
			path := filepath.Join(t.TempDir(), test.executable+".cmd")
			fixture.LookPaths[test.executable] = path
			fixture.Set(path, windowsSources[test.id].version, runner.Result{Stdout: test.current + "\n"}, nil)
			if test.prepare != nil {
				test.prepare(fixture, path)
			}
			adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture}, WindowsConfig{})

			got, err := adapter.Detect(context.Background(), mustTool(t, test.id))
			if err != nil {
				t.Fatalf("Detect() error = %v, want candidate-only failure ignored", err)
			}
			if got != (detect.Detection{Installed: true, Current: test.current}) {
				t.Fatalf("Detect() = %#v, want installed/current preserved with blank candidate", got)
			}
			if err := adapter.Verify(context.Background(), mustTool(t, test.id)); err != nil {
				t.Fatalf("Verify() error = %v, want candidate-only failure ignored", err)
			}
		})
	}
}

func TestFreshWindowsDevelopmentProfileConvergesProvidersOnceWithoutProcessPATHRefresh(t *testing.T) {
	programFiles := t.TempDir()
	nvmHome := filepath.Join(t.TempDir(), "nvm")
	nvmSymlink := filepath.Join(t.TempDir(), "nodejs")
	config := WindowsConfig{ProgramFiles: programFiles, NVMHome: nvmHome, NVMSymlink: nvmSymlink}
	paths := map[string]string{
		"git":    filepath.Join(programFiles, "Git", "cmd", "git.exe"),
		"gh":     filepath.Join(programFiles, "GitHub CLI", "gh.exe"),
		"docker": filepath.Join(programFiles, "Docker", "Docker", "resources", "bin", "docker.exe"),
		"nvm":    filepath.Join(nvmHome, "nvm.exe"), "node": filepath.Join(nvmSymlink, "node.exe"),
		"npm": filepath.Join(nvmSymlink, "npm.cmd"), "corepack": filepath.Join(nvmSymlink, "corepack.cmd"),
		"pnpm": filepath.Join(nvmSymlink, "pnpm.cmd"), "yarn": filepath.Join(nvmSymlink, "yarn.cmd"),
		"codex": filepath.Join(nvmSymlink, "codex.cmd"), "bun": filepath.Join(nvmSymlink, "bun.cmd"),
	}
	fixture := runner.NewFixture()
	for _, path := range paths {
		fixture.LookPaths[path] = path
	}
	for id, source := range windowsSources {
		path := paths[source.executable]
		fixture.Set(path, source.version, runner.Result{Stdout: string(id) + " 1.0.0\n"}, nil)
	}
	fixture.Set(paths["nvm"], []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS |\n| 26.1.0 | 24.5.0 |\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation, config)
	planned, err := plan.MergeProfiles([]profile.Profile{profile.DevelopmentProfile()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := plan.DependencyOrder(planned)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range ordered {
		if err := adapter.Install(context.Background(), tool); err != nil {
			t.Fatalf("Install(%s): %v", tool.ID, err)
		}
		if err := adapter.Verify(context.Background(), tool); err != nil {
			t.Fatalf("Verify(%s): %v", tool.ID, err)
		}
	}

	dockerInstalls := 0
	for _, command := range elevation.commands {
		if command.Command == "winget" && slicesContain(command.Args, "Docker.DockerDesktop") && command.Args[0] == "install" {
			dockerInstalls++
		}
	}
	if dockerInstalls != 1 {
		t.Fatalf("Docker Desktop install count = %d, want 1; commands = %#v", dockerInstalls, elevation.commands)
	}
	for _, command := range fixture.Commands {
		switch command.Command {
		case "git", "gh", "docker", "nvm", "node", "npm", "corepack", "pnpm", "yarn", "codex", "bun":
			t.Fatalf("fresh profile relied on stale process PATH: %#v", command)
		}
	}
}

func TestWindowsUpdatesInstalledToolFromSameWinGetSource(t *testing.T) {
	fixture := runner.NewFixture()
	fixture.LookPaths["git"] = "C:\\Program Files\\Git\\cmd\\git.exe"
	fixture.Set("git", []string{"--version"}, runner.Result{Stdout: "git version 2.48.0\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Git.Git", "--exact"}, runner.Result{Stdout: "Version: 2.49.0\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err != nil {
		t.Fatal(err)
	}
	if len(elevation.commands) != 1 {
		t.Fatalf("elevated commands = %#v, want one WinGet upgrade", elevation.commands)
	}
	want := []string{"upgrade", "--id", "Git.Git", "--exact", "--accept-package-agreements", "--accept-source-agreements"}
	if got := elevation.commands[0]; got.Command != "winget" || !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("WinGet command = %#v, want winget %#v", got, want)
	}
}

func TestWindowsRefusesToUpdateManualSystemInstallation(t *testing.T) {
	fixture := runner.NewFixture()
	path := `C:\tools\git\cmd\git.exe`
	fixture.LookPaths["git"] = path
	fixture.Set(path, []string{"--version"}, runner.Result{Stdout: "git version 2.48.0\n"}, nil)
	fixture.Set("winget", []string{"list", "--id", "Git.Git", "--exact", "--details"}, runner.Result{Stdout: "No installed package found matching input criteria.\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Git.Git", "--exact"}, runner.Result{Stdout: "Version: 2.49.0\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	detection, err := adapter.Detect(context.Background(), mustTool(t, profile.Git))
	if err != nil {
		t.Fatal(err)
	}
	if !detection.Installed || detection.Current == "" || detection.Candidate != "" {
		t.Fatalf("Detect() = %#v, want installed manual Git without update candidate", detection)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Git)); err == nil {
		t.Fatal("Update() error = nil, want provider ownership refusal")
	}
	for _, command := range elevation.commands {
		if command.Command == "winget" && len(command.Args) > 0 && command.Args[0] == "upgrade" {
			t.Fatalf("manual Git triggered WinGet upgrade: %#v", elevation.commands)
		}
	}
}

func TestWindowsDoesNotOfferStandaloneNodeForNVMUpdate(t *testing.T) {
	fixture := runner.NewFixture()
	nodePath := `C:\tools\node\node.exe`
	nvmPath := `C:\Users\johan\AppData\Roaming\nvm\nvm.exe`
	fixture.LookPaths["node"] = nodePath
	fixture.LookPaths["nvm"] = nvmPath
	fixture.Set(nodePath, []string{"--version"}, runner.Result{Stdout: "v22.1.0\n"}, nil)
	fixture.Set(nvmPath, []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS |\n| 26.1.0 | 24.5.0 |\n"}, nil)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture}, WindowsConfig{
		ProgramFiles: `C:\Program Files`, NVMHome: filepath.Dir(nvmPath), NVMSymlink: `C:\Program Files\nodejs`,
	})

	detection, err := adapter.Detect(context.Background(), mustTool(t, profile.Node))
	if err != nil {
		t.Fatal(err)
	}
	if !detection.Installed || detection.Current != "v22.1.0" || detection.Candidate != "" {
		t.Fatalf("Detect() = %#v, want standalone Node without an NVM candidate", detection)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Node)); err == nil {
		t.Fatal("Update() error = nil, want provider ownership refusal")
	}
}

func TestWindowsDockerPartialStateUsesOnePhysicalPackageRepair(t *testing.T) {
	tests := []struct {
		name    string
		missing map[profile.ToolID]bool
	}{
		{name: "Buildx missing", missing: map[profile.ToolID]bool{profile.DockerBuildx: true}},
		{name: "Compose missing", missing: map[profile.ToolID]bool{profile.DockerCompose: true}},
		{name: "both components missing", missing: map[profile.ToolID]bool{profile.DockerBuildx: true, profile.DockerCompose: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := runner.NewFixture()
			dockerPath := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
			fixture.LookPaths["docker"] = dockerPath
			fixture.Set(dockerPath, []string{"--version"}, runner.Result{Stdout: "Docker version 28.1.1\n"}, nil)
			for _, id := range []profile.ToolID{profile.DockerBuildx, profile.DockerCompose} {
				source := windowsSources[id]
				if test.missing[id] {
					component := "buildx"
					if id == profile.DockerCompose {
						component = "compose"
					}
					fixture.Set(dockerPath, source.version, runner.Result{Stderr: "docker: '" + component + "' is not a docker command\n", ExitCode: 1}, errors.New("exit status 1"))
				} else {
					fixture.Set(dockerPath, source.version, runner.Result{Stdout: string(id) + " 1.0.0\n"}, nil)
				}
			}
			fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
			elevation := &windowsFixtureElevation{fixture: fixture}
			adapter := NewWindowsAdapter(fixture, elevation)

			ids := []profile.ToolID{profile.Docker, profile.DockerBuildx, profile.DockerCompose}
			detections := make(map[profile.ToolID]detect.Detection, len(ids))
			for _, id := range ids {
				tool := mustTool(t, id)
				detection, err := adapter.Detect(context.Background(), tool)
				if err != nil {
					t.Fatalf("Detect(%s): %v", id, err)
				}
				detections[id] = detection
			}
			for _, id := range ids {
				tool := mustTool(t, id)
				if detections[id].Installed {
					if err := adapter.Update(context.Background(), tool); err != nil {
						t.Fatalf("Update(%s): %v", id, err)
					}
				} else if err := adapter.Install(context.Background(), tool); err != nil {
					t.Fatalf("Install(%s): %v", id, err)
				}
			}

			want := []string{"install", "--id", "Docker.DockerDesktop", "--exact", "--accept-package-agreements", "--accept-source-agreements", "--force"}
			if len(elevation.commands) != 1 || elevation.commands[0].Command != "winget" || !reflect.DeepEqual(elevation.commands[0].Args, want) {
				t.Fatalf("Docker convergence commands = %#v, want one forced physical-package repair %#v", elevation.commands, want)
			}
		})
	}
}

func TestWindowsDockerRepairIsIndependentOfDetectionOrder(t *testing.T) {
	fixture := runner.NewFixture()
	dockerPath := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.LookPaths["docker"] = dockerPath
	fixture.Set(dockerPath, []string{"--version"}, runner.Result{Stdout: "Docker version 28.1.1\n"}, nil)
	fixture.Set(dockerPath, []string{"buildx", "version"}, runner.Result{Stderr: "docker: 'buildx' is not a docker command\n", ExitCode: 1}, errors.New("exit status 1"))
	fixture.Set(dockerPath, []string{"compose", "version"}, runner.Result{Stdout: "Docker Compose version v2.36.0\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	for _, id := range []profile.ToolID{profile.DockerCompose, profile.DockerBuildx, profile.Docker} {
		if _, err := adapter.Detect(context.Background(), mustTool(t, id)); err != nil {
			t.Fatalf("Detect(%s): %v", id, err)
		}
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Docker)); err != nil {
		t.Fatal(err)
	}

	want := []string{"install", "--id", "Docker.DockerDesktop", "--exact", "--accept-package-agreements", "--accept-source-agreements", "--force"}
	if len(elevation.commands) != 1 || !reflect.DeepEqual(elevation.commands[0].Args, want) {
		t.Fatalf("Docker convergence commands = %#v, want fresh forced repair %#v", elevation.commands, want)
	}
}

func TestWindowsDockerConvergenceRechecksHealthAfterRepeatedDetection(t *testing.T) {
	fixture := runner.NewFixture()
	dockerPath := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
	fixture.LookPaths["docker"] = dockerPath
	fixture.Set(dockerPath, []string{"--version"}, runner.Result{Stdout: "Docker version 28.1.1\n"}, nil)
	fixture.Set(dockerPath, []string{"buildx", "version"}, runner.Result{Stderr: "docker: 'buildx' is not a docker command\n", ExitCode: 1}, errors.New("exit status 1"))
	fixture.Set(dockerPath, []string{"compose", "version"}, runner.Result{Stdout: "Docker Compose version v2.36.0\n"}, nil)
	fixture.Set("winget", []string{"show", "--id", "Docker.DockerDesktop", "--exact"}, runner.Result{Stdout: "Version: 4.42.0\n"}, nil)
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	for _, id := range []profile.ToolID{profile.Docker, profile.DockerBuildx} {
		if _, err := adapter.Detect(context.Background(), mustTool(t, id)); err != nil {
			t.Fatalf("initial Detect(%s): %v", id, err)
		}
	}
	fixture.Set(dockerPath, []string{"buildx", "version"}, runner.Result{Stdout: "github.com/docker/buildx v0.24.0\n"}, nil)
	if _, err := adapter.Detect(context.Background(), mustTool(t, profile.DockerBuildx)); err != nil {
		t.Fatalf("repeated Detect(buildx): %v", err)
	}
	if err := adapter.Update(context.Background(), mustTool(t, profile.Docker)); err != nil {
		t.Fatal(err)
	}

	want := []string{"upgrade", "--id", "Docker.DockerDesktop", "--exact", "--accept-package-agreements", "--accept-source-agreements"}
	if len(elevation.commands) != 1 || !reflect.DeepEqual(elevation.commands[0].Args, want) {
		t.Fatalf("Docker convergence commands = %#v, want fresh healthy upgrade %#v", elevation.commands, want)
	}
}

func TestWindowsDefaultElevationUsesFileHelperWithSeparateArguments(t *testing.T) {
	fixture := runner.NewFixture()
	elevation := windowsElevation{runner: fixture}
	wingetArgs := []string{"install", "--id", "Git.Git", "--exact"}

	if _, err := elevation.RunElevated(context.Background(), "winget", wingetArgs...); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Commands) != 1 {
		t.Fatalf("commands = %#v, want one PowerShell invocation", fixture.Commands)
	}
	got := fixture.Commands[0]
	wantPrefix := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}
	if got.Command != "powershell.exe" || len(got.Args) < len(wantPrefix)+2 || !reflect.DeepEqual(got.Args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("default elevation = %#v, want PowerShell -File helper", got)
	}
	helper := got.Args[len(wantPrefix)]
	wantPassedArgs := append([]string{"winget"}, wingetArgs...)
	if !reflect.DeepEqual(got.Args[len(wantPrefix)+1:], wantPassedArgs) {
		t.Fatalf("PowerShell arguments = %#v, want %#v", got.Args[len(wantPrefix)+1:], wantPassedArgs)
	}
	for _, arg := range got.Args {
		if arg == "-Command" {
			t.Fatalf("default elevation used -Command instead of -File: %#v", got)
		}
	}
	if _, err := os.Stat(helper); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("elevation helper cleanup error = %v", err)
	}
}

func TestWindowsNodeInstallAndUpdateUseElevatedNVMSequence(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*runner.Fixture)
		run     func(Adapter) error
	}{
		{
			name: "install",
			run: func(adapter Adapter) error {
				return adapter.Install(context.Background(), mustTool(t, profile.Node))
			},
		},
		{
			name: "update",
			prepare: func(fixture *runner.Fixture) {
				fixture.LookPaths["node"] = "C:\\Program Files\\nodejs\\node.exe"
				fixture.Set("node", []string{"--version"}, runner.Result{Stdout: "v24.0.0\n"}, nil)
			},
			run: func(adapter Adapter) error {
				return adapter.Update(context.Background(), mustTool(t, profile.Node))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := runner.NewFixture()
			nvmPath := `C:\Users\johan\AppData\Roaming\nvm\nvm.exe`
			fixture.LookPaths["nvm"] = nvmPath
			fixture.Set(nvmPath, []string{"list", "available"}, runner.Result{Stdout: "| CURRENT | LTS |\n| 26.1.0 | 24.5.0 |\n"}, nil)
			if test.prepare != nil {
				test.prepare(fixture)
			}
			elevation := &windowsFixtureElevation{fixture: fixture}

			if err := test.run(NewWindowsAdapter(fixture, elevation, WindowsConfig{
				ProgramFiles: `C:\Program Files`, NVMHome: filepath.Dir(nvmPath), NVMSymlink: `C:\Program Files\nodejs`,
			})); err != nil {
				t.Fatal(err)
			}
			want := []runner.Command{
				{Command: nvmPath, Args: []string{"install", "lts"}},
				{Command: nvmPath, Args: []string{"use", "lts"}},
			}
			if !reflect.DeepEqual(elevation.commands, want) {
				t.Fatalf("elevated commands = %#v, want %#v", elevation.commands, want)
			}
		})
	}
}

func TestWindowsUsesElevationOnlyForSystemChanges(t *testing.T) {
	fixture := runner.NewFixture()
	npmPath := `C:\Program Files\nodejs\npm.cmd`
	fixture.LookPaths["npm"] = npmPath
	elevation := &windowsFixtureElevation{fixture: fixture}
	adapter := NewWindowsAdapter(fixture, elevation)

	if err := adapter.Install(context.Background(), mustTool(t, profile.Codex)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Install(context.Background(), mustTool(t, profile.Bun)); err != nil {
		t.Fatal(err)
	}
	if len(elevation.commands) != 0 {
		t.Fatalf("user installers used elevation: %#v", elevation.commands)
	}
	assertHasCommand(t, fixture.Commands, npmPath, "install", "--global", "@openai/codex@latest")
	assertHasCommand(t, fixture.Commands, npmPath, "install", "--global", "--ignore-scripts=false", "--bin-links=true", "bun@latest")
	for _, command := range fixture.Commands {
		if command.Command == "bash" || command.Command == "sh" || command.Command == "nvm" {
			t.Fatalf("Windows adapter invoked a Linux command: %#v", command)
		}
	}
}

func TestWindowsInstallerFailurePreservesExitStatus(t *testing.T) {
	fixture := runner.NewFixture()
	wantErr := &windowsExitError{status: 23}
	npmPath := `C:\Program Files\nodejs\npm.cmd`
	fixture.LookPaths["npm"] = npmPath
	fixture.Set(npmPath, []string{"install", "--global", "@openai/codex@latest"}, runner.Result{ExitCode: 23}, wantErr)
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	err := adapter.Install(context.Background(), mustTool(t, profile.Codex))
	if !errors.Is(err, wantErr) || err.(*windowsExitError).ExitCode() != 23 {
		t.Fatalf("Install() error = %#v, want unchanged exit status 23", err)
	}
}

func TestWindowsRejectsToolsWithoutNativeInstallationPath(t *testing.T) {
	fixture := runner.NewFixture()
	adapter := NewWindowsAdapter(fixture, &windowsFixtureElevation{fixture: fixture})

	err := adapter.Install(context.Background(), profile.Tool{ID: "linux-only", Name: "Linux only"})
	if err == nil {
		t.Fatal("Install() error = nil, want unsupported-provider error")
	}
}

type windowsFixtureElevation struct {
	fixture  *runner.Fixture
	commands []runner.Command
}

func (e *windowsFixtureElevation) RunElevated(ctx context.Context, command string, args ...string) (runner.Result, error) {
	e.commands = append(e.commands, runner.Command{Command: command, Args: append([]string(nil), args...)})
	return e.fixture.Run(ctx, command, args...)
}

type windowsExitError struct{ status int }

func (e *windowsExitError) Error() string { return "Windows installer failed" }
func (e *windowsExitError) ExitCode() int { return e.status }

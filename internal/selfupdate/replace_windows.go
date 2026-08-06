//go:build windows

package selfupdate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const replacementScript = `param(
    [int]$ParentPid,
    [string]$Staged,
    [string]$Destination,
    [string]$ScriptPath
)

$ErrorActionPreference = 'Stop'
try {
    Wait-Process -Id $ParentPid -ErrorAction SilentlyContinue
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        try {
            Move-Item -LiteralPath $Staged -Destination $Destination -Force -ErrorAction Stop
            break
        } catch {
            if ($attempt -eq 59) {
                throw
            }
            Start-Sleep -Milliseconds 200
        }
    }
} finally {
    Remove-Item -LiteralPath $ScriptPath -Force -ErrorAction SilentlyContinue
}
`

func replaceExecutable(staged, destination string) (bool, error) {
	script, err := os.CreateTemp("", "jb-update-*.ps1")
	if err != nil {
		return false, fmt.Errorf("create Windows replacement helper: %w", err)
	}
	scriptPath := script.Name()
	removeScript := true
	defer func() {
		_ = script.Close()
		if removeScript {
			_ = os.Remove(scriptPath)
		}
	}()
	if _, err := script.WriteString(replacementScript); err != nil {
		return false, fmt.Errorf("write Windows replacement helper: %w", err)
	}
	if err := script.Close(); err != nil {
		return false, fmt.Errorf("close Windows replacement helper: %w", err)
	}

	command := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-ParentPid", strconv.Itoa(os.Getpid()),
		"-Staged", staged,
		"-Destination", destination,
		"-ScriptPath", scriptPath,
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	if err := command.Start(); err != nil {
		return false, fmt.Errorf("start Windows replacement helper: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		removeScript = false
		return true, fmt.Errorf("detach Windows replacement helper: %w", err)
	}
	removeScript = false
	return true, nil
}

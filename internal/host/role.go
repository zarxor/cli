// Package host classifies the current machine for automatic tool profiles.
package host

import (
	"os"
	"runtime"
	"strings"
)

// Role is the broad machine shape used to choose an automatic tool profile.
type Role string

const (
	Desktop Role = "desktop"
	Server  Role = "server"
	Unknown Role = "unknown"
)

// Facts contains the small set of host signals used by Classify. Keeping the
// classifier separate from filesystem and registry access makes the policy
// deterministic and easy to test.
type Facts struct {
	OS                 string
	Environment        map[string]string
	DesktopIndicators  []string
	ServerIndicators   []string
	WindowsProductName string
}

// Detection is the result shown when an automatic profile is applied.
type Detection struct {
	Role   Role
	Reason string
}

// Detect gathers host signals using the current process environment and the
// platform-specific helpers, then classifies them.
func Detect() (Detection, error) {
	return Classify(Facts{
		OS:                 runtime.GOOS,
		Environment:        currentEnvironment(),
		DesktopIndicators:  desktopEnvironmentIndicators(),
		WindowsProductName: windowsProductName(),
	}), nil
}

// Classify applies conservative, explainable heuristics. A graphical session
// or installed desktop environment wins over weak remote-shell signals. A
// Unix host with no desktop evidence defaults to Server because that avoids
// applying desktop-only tools to a headless machine.
func Classify(facts Facts) Detection {
	osName := strings.ToLower(strings.TrimSpace(facts.OS))
	if osName == "windows" {
		if strings.Contains(strings.ToLower(facts.WindowsProductName), "server") {
			return Detection{Role: Server, Reason: "Windows Server product detected"}
		}
		return Detection{Role: Desktop, Reason: "Windows desktop host"}
	}

	if hasGraphicalSession(facts.Environment) {
		return Detection{Role: Desktop, Reason: "active graphical session detected"}
	}
	if len(facts.DesktopIndicators) > 0 {
		return Detection{Role: Desktop, Reason: "desktop environment detected"}
	}
	if len(facts.ServerIndicators) > 0 || hasServerEnvironment(facts.Environment) {
		return Detection{Role: Server, Reason: "server environment detected"}
	}
	if hasValue(facts.Environment, "SSH_CONNECTION", "SSH_TTY") {
		return Detection{Role: Server, Reason: "remote shell detected without a graphical session"}
	}
	if osName == "linux" || osName == "freebsd" || osName == "openbsd" || osName == "netbsd" {
		return Detection{Role: Server, Reason: "no graphical desktop indicators found"}
	}
	return Detection{Role: Unknown, Reason: "host type could not be determined"}
}

func currentEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[key] = value
		}
	}
	return environment
}

func hasGraphicalSession(environment map[string]string) bool {
	return hasValue(environment,
		"DISPLAY",
		"WAYLAND_DISPLAY",
		"XDG_CURRENT_DESKTOP",
		"XDG_SESSION_DESKTOP",
		"DESKTOP_SESSION",
		"GDMSESSION",
	)
}

func hasServerEnvironment(environment map[string]string) bool {
	return hasValue(environment,
		"KUBERNETES_SERVICE_HOST",
		"container",
		"CONTAINER",
		"containerized",
	)
}

func hasValue(environment map[string]string, keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(environment[key]) != "" {
			return true
		}
	}
	return false
}

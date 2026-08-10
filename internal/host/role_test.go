package host_test

import (
	"testing"

	"github.com/zarxor/cli/internal/host"
)

func TestClassifyDetectsDesktopFromGraphicalSession(t *testing.T) {
	got := host.Classify(host.Facts{
		OS:          "linux",
		Environment: map[string]string{"XDG_CURRENT_DESKTOP": "GNOME"},
	})
	if got.Role != host.Desktop || got.Reason != "active graphical session detected" {
		t.Fatalf("classification = %#v, want desktop graphical-session detection", got)
	}
}

func TestClassifyDetectsDesktopFromInstalledEnvironment(t *testing.T) {
	got := host.Classify(host.Facts{OS: "linux", DesktopIndicators: []string{"GNOME Shell"}})
	if got.Role != host.Desktop {
		t.Fatalf("classification = %#v, want desktop", got)
	}
}

func TestClassifyDetectsServerFromServerSignals(t *testing.T) {
	got := host.Classify(host.Facts{
		OS:                "linux",
		Environment:       map[string]string{"KUBERNETES_SERVICE_HOST": "10.0.0.1"},
		DesktopIndicators: nil,
	})
	if got.Role != host.Server || got.Reason != "server environment detected" {
		t.Fatalf("classification = %#v, want server environment detection", got)
	}
}

func TestClassifyTreatsHeadlessUnixAsServer(t *testing.T) {
	got := host.Classify(host.Facts{OS: "linux"})
	if got.Role != host.Server {
		t.Fatalf("classification = %#v, want server", got)
	}
}

func TestClassifyDetectsWindowsServerProduct(t *testing.T) {
	got := host.Classify(host.Facts{OS: "windows", WindowsProductName: "Windows Server 2025 Standard"})
	if got.Role != host.Server {
		t.Fatalf("classification = %#v, want server", got)
	}
}

func TestClassifyDefaultsWindowsToDesktop(t *testing.T) {
	got := host.Classify(host.Facts{OS: "windows", WindowsProductName: "Windows 11 Pro"})
	if got.Role != host.Desktop {
		t.Fatalf("classification = %#v, want desktop", got)
	}
}

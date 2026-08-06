// Package platform identifies the supported host operating-system families.
package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type OS string

const (
	Debian  OS = "debian"
	Arch    OS = "arch"
	Windows OS = "windows"
)

// Detect identifies the current host from the Go runtime and, on Linux, the
// standard os-release file.
func Detect() (OS, error) {
	var release []byte
	if runtime.GOOS == "linux" {
		var err error
		release, err = os.ReadFile("/etc/os-release")
		if err != nil {
			return "", fmt.Errorf("read /etc/os-release: %w", err)
		}
	}
	return DetectFrom(runtime.GOOS, release)
}

// DetectFrom identifies an OS family from runtime and os-release values. It
// is exported to keep platform fixtures deterministic in tests.
func DetectFrom(goos string, release []byte) (OS, error) {
	if goos == "windows" {
		return Windows, nil
	}
	if goos != "linux" {
		return "", fmt.Errorf("unsupported operating system %q", goos)
	}

	fields := parseOSRelease(string(release))
	identifiers := strings.Fields(fields["ID"] + " " + fields["ID_LIKE"])
	for _, identifier := range identifiers {
		switch identifier {
		case "debian", "ubuntu":
			return Debian, nil
		case "arch":
			return Arch, nil
		}
	}
	return "", fmt.Errorf("unsupported Linux distribution %q", fields["ID"])
}

func parseOSRelease(release string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(release, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, "\"")
	}
	return fields
}

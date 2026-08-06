// Package detect contains shared live-installation detection values.
package detect

import "strings"

type Detection struct {
	Installed bool
	Current   string
	Candidate string
}

// ParseVersion returns the first non-empty line emitted by a version command.
// Some tools emit normal version output on stderr, so stdout takes precedence
// but stderr is used when stdout is empty.
func ParseVersion(stdout, stderr string) string {
	if version := firstLine(stdout); version != "" {
		return version
	}
	return firstLine(stderr)
}

func firstLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

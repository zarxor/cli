package detect

import "testing"

func TestParseVersionUsesStdout(t *testing.T) {
	got := ParseVersion("tool version 1.2.3\n", "warning: old config\n")
	if got != "tool version 1.2.3" {
		t.Fatalf("ParseVersion() = %q", got)
	}
}

func TestParseVersionUsesStderrWhenStdoutIsEmpty(t *testing.T) {
	got := ParseVersion("", "tool version 4.5.6\n")
	if got != "tool version 4.5.6" {
		t.Fatalf("ParseVersion() = %q", got)
	}
}

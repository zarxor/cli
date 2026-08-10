package skills

import "testing"

func TestParseScopeMode(t *testing.T) {
	tests := []struct {
		value string
		want  ScopeMode
	}{
		{value: "global", want: ScopeModeGlobal},
		{value: " PROJECT ", want: ScopeModeProject},
		{value: "choose", want: ScopeModeChoose},
	}
	for _, test := range tests {
		got, err := ParseScopeMode(test.value)
		if err != nil {
			t.Fatalf("ParseScopeMode(%q) error = %v", test.value, err)
		}
		if got != test.want {
			t.Fatalf("ParseScopeMode(%q) = %q, want %q", test.value, got, test.want)
		}
	}
	if _, err := ParseScopeMode("workspace"); err == nil {
		t.Fatal("ParseScopeMode(workspace) error = nil")
	}
}

func TestScopeForMode(t *testing.T) {
	if got := ScopeForMode(ScopeModeGlobal); got != ScopeUser {
		t.Fatalf("ScopeForMode(global) = %q, want %q", got, ScopeUser)
	}
	if got := ScopeForMode(ScopeModeProject); got != ScopeProject {
		t.Fatalf("ScopeForMode(project) = %q, want %q", got, ScopeProject)
	}
}

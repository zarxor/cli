package skills

import "strings"

// ParseScopeMode accepts the install scope choices exposed by the CLI.
func ParseScopeMode(value string) (ScopeMode, error) {
	mode := ScopeMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case ScopeModeGlobal, ScopeModeProject, ScopeModeChoose:
		return mode, nil
	default:
		return "", unknownScopeModeError(value)
	}
}

// ScopeForMode maps the user-facing install scope to the persisted scope.
func ScopeForMode(mode ScopeMode) Scope {
	if mode == ScopeModeProject {
		return ScopeProject
	}
	return ScopeUser
}

package skills

import "fmt"

func unknownTargetError(value string) error {
	return fmt.Errorf("unknown skill target %q (want codex, claude, copilot, agents, or all)", value)
}

func unknownScopeError(value string) error {
	return fmt.Errorf("unknown skill scope %q (want user or project)", value)
}

func unknownScopeModeError(value string) error {
	return fmt.Errorf("unknown skill scope mode %q (want global, project, or choose)", value)
}

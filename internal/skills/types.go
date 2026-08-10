// Package skills manages portable Agent Skill bundles for supported agents.
package skills

import "net/http"

// SkillID is the stable catalog identifier used by selection and lifecycle
// operations. It is deliberately separate from a display name.
type SkillID string

// ScopeMode controls how an install chooses its ownership boundary.
type ScopeMode string

const (
	ScopeModeGlobal  ScopeMode = "global"
	ScopeModeProject ScopeMode = "project"
	ScopeModeChoose  ScopeMode = "choose"
)

// CatalogEntry is one skill deliberately made available by the product
// catalog. The command line never accepts a source directly; Source is an
// implementation detail of an available entry.
type CatalogEntry struct {
	ID          SkillID
	Creator     string
	Name        string
	Description string
	Source      string
	Target      Target
	Scope       Scope
}

// CatalogStatus is the local and remote state of one available skill.
type CatalogStatus struct {
	Entry              CatalogEntry
	Installed          bool
	PartiallyInstalled bool
	Managed            bool
	UpdateAvailable    bool
	InstalledDigest    string
	CandidateDigest    string
	Message            string
}

// CatalogProgress receives one callback after each catalog entry is checked.
// It is kept outside the renderer so catalog checks remain testable and the
// CLI can use the same progress bar as tool discovery.
type CatalogProgress func(completed, total int) error

// CatalogOperationOptions controls catalog-backed installation and updates.
type CatalogOperationOptions struct {
	DryRun   bool
	Progress func(message string) error
}

// Target identifies the agent that discovers an installed skill.
type Target string

const (
	TargetCodex   Target = "codex"
	TargetClaude  Target = "claude"
	TargetCopilot Target = "copilot"
	TargetAgents  Target = "agents"
	TargetAll     Target = "all"
)

// ParseTarget accepts the stable command-line target names.
func ParseTarget(value string) (Target, error) {
	target := Target(value)
	switch target {
	case TargetCodex, TargetClaude, TargetCopilot, TargetAgents, TargetAll:
		return target, nil
	default:
		return "", unknownTargetError(value)
	}
}

// Targets returns the concrete targets represented by a selector.
func Targets(target Target) []Target {
	if target == TargetAll {
		return []Target{TargetCodex, TargetClaude, TargetCopilot}
	}
	return []Target{target}
}

// Scope identifies whether a skill is shared by the user or checked into a project.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// ParseScope accepts the stable command-line scope names.
func ParseScope(value string) (Scope, error) {
	scope := Scope(value)
	switch scope {
	case ScopeUser, ScopeProject:
		return scope, nil
	default:
		return "", unknownScopeError(value)
	}
}

// Environment makes filesystem and network behavior injectable for tests.
type Environment struct {
	HomeDir    string
	WorkDir    string
	ConfigDir  string
	CodexHome  string
	HTTPClient *http.Client
}

// Manager owns skill installation, discovery, verification, and lifecycle operations.
type Manager struct {
	env        Environment
	httpClient *http.Client
	catalog    []CatalogEntry
}

// Metadata is the required identity exposed by a SKILL.md frontmatter block.
type Metadata struct {
	Name        string
	Description string
}

// Info describes a skill found in a target directory.
type Info struct {
	Metadata
	Path    string
	Target  Target
	Scope   Scope
	Source  string
	Digest  string
	Managed bool
	Valid   bool
	Error   string
}

// Result is a lifecycle or verification result for one skill.
type Result struct {
	Name        string
	Description string
	Target      Target
	Scope       Scope
	Path        string
	Status      string
	Message     string
}

// InstallOptions controls skill installation.
type InstallOptions struct {
	Target       Target
	Scope        Scope
	DryRun       bool
	Force        bool
	ExpectedName string
}

// UpdateOptions controls managed skill updates.
type UpdateOptions struct {
	Target Target
	Scope  Scope
	Names  []string
	DryRun bool
}

// RemoveOptions controls managed skill removal.
type RemoveOptions struct {
	Target Target
	Scope  Scope
	Names  []string
	DryRun bool
}

// InspectOptions selects a target and scope for read-only operations.
type InspectOptions struct {
	Target Target
	Scope  Scope
	Names  []string
}

// Lockfile records the source and digest for skills managed by jb.
type Lockfile struct {
	Version int         `json:"version"`
	Skills  []LockEntry `json:"skills"`
}

// LockEntry is the provenance record for one installed skill.
type LockEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
	Target      Target `json:"target"`
	Scope       Scope  `json:"scope"`
	Digest      string `json:"digest"`
}

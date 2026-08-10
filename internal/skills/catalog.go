package skills

import (
	"fmt"
	"strings"
)

// Catalog is the explicit list of skills exposed by jb skills install and
// jb skills update. Keep this list intentional: a skill is not available just
// because it exists somewhere on the internet or on the host.
//
// The catalog contains the stable skills we intentionally expose from the
// supported upstream collections. In-progress and deprecated upstream
// entries are kept out of the install surface until they are promoted.
var Catalog = []CatalogEntry{
	{ID: "ask-matt", Creator: "Matt Pocock", Name: "ask-matt", Description: "Get guidance from Matt Pocock on a difficult engineering problem.", Source: "github:mattpocock/skills/skills/engineering/ask-matt@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "code-review", Creator: "Matt Pocock", Name: "code-review", Description: "Review changes against project standards and the requested specification.", Source: "github:mattpocock/skills/skills/engineering/code-review@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "codebase-design", Creator: "Matt Pocock", Name: "codebase-design", Description: "Design deeper, clearer module boundaries and interfaces.", Source: "github:mattpocock/skills/skills/engineering/codebase-design@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "diagnosing-bugs", Creator: "Matt Pocock", Name: "diagnosing-bugs", Description: "Investigate hard bugs and performance regressions systematically.", Source: "github:mattpocock/skills/skills/engineering/diagnosing-bugs@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "domain-modeling", Creator: "Matt Pocock", Name: "domain-modeling", Description: "Sharpen domain terminology and capture a shared model.", Source: "github:mattpocock/skills/skills/engineering/domain-modeling@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "grill-with-docs", Creator: "Matt Pocock", Name: "grill-with-docs", Description: "Stress-test a plan against high-trust documentation.", Source: "github:mattpocock/skills/skills/engineering/grill-with-docs@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "implement", Creator: "Matt Pocock", Name: "implement", Description: "Turn a scoped request into a tested implementation.", Source: "github:mattpocock/skills/skills/engineering/implement@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "improve-codebase-architecture", Creator: "Matt Pocock", Name: "improve-codebase-architecture", Description: "Find and apply opportunities to improve a codebase architecture.", Source: "github:mattpocock/skills/skills/engineering/improve-codebase-architecture@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "prototype", Creator: "Matt Pocock", Name: "prototype", Description: "Build a small prototype to answer a design question.", Source: "github:mattpocock/skills/skills/engineering/prototype@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "research", Creator: "Matt Pocock", Name: "research", Description: "Investigate a question using high-trust primary sources.", Source: "github:mattpocock/skills/skills/engineering/research@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "resolving-merge-conflicts", Creator: "Matt Pocock", Name: "resolving-merge-conflicts", Description: "Resolve an in-progress Git merge or rebase conflict safely.", Source: "github:mattpocock/skills/skills/engineering/resolving-merge-conflicts@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "setup-matt-pocock-skills", Creator: "Matt Pocock", Name: "setup-matt-pocock-skills", Description: "Set up the broader Matt Pocock skill collection.", Source: "github:mattpocock/skills/skills/engineering/setup-matt-pocock-skills@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "tdd", Creator: "Matt Pocock", Name: "tdd", Description: "Develop changes test-first with a red-green-refactor loop.", Source: "github:mattpocock/skills/skills/engineering/tdd@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "to-spec", Creator: "Matt Pocock", Name: "to-spec", Description: "Turn an idea into a clear implementation specification.", Source: "github:mattpocock/skills/skills/engineering/to-spec@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "to-tickets", Creator: "Matt Pocock", Name: "to-tickets", Description: "Break a specification into actionable engineering tickets.", Source: "github:mattpocock/skills/skills/engineering/to-tickets@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "triage", Creator: "Matt Pocock", Name: "triage", Description: "Sort incoming work by impact, urgency, and next action.", Source: "github:mattpocock/skills/skills/engineering/triage@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "wayfinder", Creator: "Matt Pocock", Name: "wayfinder", Description: "Navigate an unfamiliar codebase and find the right place to change.", Source: "github:mattpocock/skills/skills/engineering/wayfinder@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "wizard", Creator: "Matt Pocock", Name: "wizard", Description: "Generate a guided workflow for steps only the user can perform.", Source: "github:mattpocock/skills/skills/engineering/wizard@main", Target: TargetCodex, Scope: ScopeUser},

	{ID: "grill-me", Creator: "Matt Pocock", Name: "grill-me", Description: "Stress-test a decision or idea with focused questions.", Source: "github:mattpocock/skills/skills/productivity/grill-me@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "grilling", Creator: "Matt Pocock", Name: "grilling", Description: "Relentlessly challenge a plan, decision, or idea.", Source: "github:mattpocock/skills/skills/productivity/grilling@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "handoff", Creator: "Matt Pocock", Name: "handoff", Description: "Prepare a clear, useful handoff for another person or agent.", Source: "github:mattpocock/skills/skills/productivity/handoff@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "teach", Creator: "Matt Pocock", Name: "teach", Description: "Explain a technical subject at the learner's level.", Source: "github:mattpocock/skills/skills/productivity/teach@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "to-questionnaire", Creator: "Matt Pocock", Name: "to-questionnaire", Description: "Turn an ambiguous request into a focused questionnaire.", Source: "github:mattpocock/skills/skills/productivity/to-questionnaire@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "wait-what", Creator: "Matt Pocock", Name: "wait-what", Description: "Pause and clarify confusing requirements before acting.", Source: "github:mattpocock/skills/skills/productivity/wait-what@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "writing-for-agents", Creator: "Matt Pocock", Name: "writing-for-agents", Description: "Write durable instructions and documents for agents.", Source: "github:mattpocock/skills/skills/productivity/writing-for-agents@main", Target: TargetCodex, Scope: ScopeUser},

	{ID: "git-guardrails-claude-code", Creator: "Matt Pocock", Name: "git-guardrails-claude-code", Description: "Add safer Git guardrails for Claude Code workflows.", Source: "github:mattpocock/skills/skills/misc/git-guardrails-claude-code@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "migrate-to-shoehorn", Creator: "Matt Pocock", Name: "migrate-to-shoehorn", Description: "Migrate a project to the Shoehorn workflow.", Source: "github:mattpocock/skills/skills/misc/migrate-to-shoehorn@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "scaffold-exercises", Creator: "Matt Pocock", Name: "scaffold-exercises", Description: "Scaffold exercises for a learning or coding session.", Source: "github:mattpocock/skills/skills/misc/scaffold-exercises@main", Target: TargetCodex, Scope: ScopeUser},
	{ID: "setup-pre-commit", Creator: "Matt Pocock", Name: "setup-pre-commit", Description: "Set up pre-commit checks for a project.", Source: "github:mattpocock/skills/skills/misc/setup-pre-commit@main", Target: TargetCodex, Scope: ScopeUser},

	{ID: "impeccable", Creator: "Paul Bakaus", Name: "impeccable", Description: "Design, redesign, critique, and polish frontend interfaces.", Source: "github:pbakaus/impeccable/.agents/skills/impeccable@main", Target: TargetCodex, Scope: ScopeUser},
}

func normalizeCatalog(entries []CatalogEntry) ([]CatalogEntry, error) {
	return normalizeCatalogEntries(entries, false)
}

func normalizeCatalogEntries(entries []CatalogEntry, allowScopedDuplicates bool) ([]CatalogEntry, error) {
	result := make([]CatalogEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, original := range entries {
		entry := original
		entry.ID = SkillID(strings.ToLower(strings.TrimSpace(string(entry.ID))))
		if entry.ID == "" {
			return nil, fmt.Errorf("skill catalog entry is missing an id")
		}
		if !skillNamePattern.MatchString(string(entry.ID)) {
			return nil, fmt.Errorf("skill catalog id %q must use lowercase letters, numbers, and hyphens", entry.ID)
		}
		if strings.TrimSpace(entry.Name) == "" {
			entry.Name = string(entry.ID)
		}
		entry.Name = strings.TrimSpace(entry.Name)
		entry.Creator = strings.TrimSpace(entry.Creator)
		entry.Description = strings.TrimSpace(entry.Description)
		entry.Source = strings.TrimSpace(entry.Source)
		if entry.Source == "" {
			return nil, fmt.Errorf("skill catalog entry %q is missing a source", entry.ID)
		}
		if entry.Target == "" {
			entry.Target = TargetCodex
		}
		if entry.Scope == "" {
			entry.Scope = ScopeUser
		}
		if entry.Target != TargetAll {
			if _, err := ParseTarget(string(entry.Target)); err != nil {
				return nil, err
			}
		}
		if _, err := ParseScope(string(entry.Scope)); err != nil {
			return nil, err
		}
		key := string(entry.ID)
		if allowScopedDuplicates {
			key += "\x00" + string(entry.Scope) + "\x00" + string(entry.Target)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("skill catalog contains duplicate id %q", entry.ID)
		}
		seen[key] = struct{}{}
		result = append(result, entry)
	}
	return result, nil
}

func cloneCatalog(entries []CatalogEntry) []CatalogEntry {
	return append([]CatalogEntry(nil), entries...)
}

// ResolveCatalog resolves a requested subset, preserving catalog order.
func ResolveCatalog(entries []CatalogEntry, ids []SkillID) ([]CatalogEntry, error) {
	normalized, err := normalizeCatalog(entries)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return normalized, nil
	}
	requested := make(map[SkillID]struct{}, len(ids))
	for _, rawID := range ids {
		id := SkillID(strings.ToLower(strings.TrimSpace(string(rawID))))
		if id == "" {
			return nil, fmt.Errorf("skill name cannot be empty")
		}
		requested[id] = struct{}{}
	}
	result := make([]CatalogEntry, 0, len(requested))
	for _, entry := range normalized {
		if _, ok := requested[entry.ID]; !ok {
			continue
		}
		result = append(result, entry)
		delete(requested, entry.ID)
	}
	for id := range requested {
		return nil, fmt.Errorf("unknown skill %q", id)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("skill selection is empty")
	}
	return result, nil
}

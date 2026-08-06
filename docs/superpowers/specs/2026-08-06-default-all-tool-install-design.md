# Default-All Tool Install Design

## Purpose

Make `jb tools install` useful without selection flags by treating an
unqualified install as a request for the complete supported tool catalog.

## Command Behavior

Running:

```text
jb tools install
```

resolves every entry in `tools.Catalog`, preserving catalog order. The existing
interactive selection screen is then shown with every eligible tool preselected.
The user may deselect tools before continuing.

Running:

```text
jb tools install --yes
```

uses the same complete-catalog plan and accepts every eligible tool without
showing the interactive selection screen.

Existing scoped forms retain their current behavior:

- `--profiles=<name>[,<name>...]` selects the named profiles.
- `--only=<tool>[,<tool>...]` selects those tools and required dependencies.
- Combining `--profiles` and `--only` narrows the profile selection.
- Empty flag values and unknown profile or tool names remain errors.
- `--dry-run` continues to show the resulting plan without host mutation.

Already-installed tools retain the existing convergence behavior: install plans
update them when a newer candidate is available and report them as up to date
when versions match.

## Architecture

The command parser will stop rejecting install requests that omit both
`--profiles` and `--only`. It will pass an empty scope to `ToolsService`, just as
the unqualified update command does today.

The service-level `requestedTools` function will own the defaulting rule for
both actions. When profiles and `--only` are both empty, it returns a defensive
copy of `tools.Catalog`. Scoped requests continue through the existing profile
planner or tool resolver. Keeping the rule in the service prevents command-line
parsing from duplicating catalog and dependency-planning logic.

No new flag, profile, catalog entry, or install execution path is introduced.

## Error Handling

Default resolution itself cannot produce an empty selection while the catalog
contains supported tools. Adapter loading, detection, interactive selection,
dependency ordering, installation, updating, and verification errors continue
through their existing paths and messages.

Explicit user cancellation or deselection of every item continues to produce no
host mutation through the existing selection behavior.

## Testing

Command-layer tests will prove that `tools install` without selection flags
reaches `ToolsService` with an empty scope instead of returning the former
validation error.

Service-level tests will prove that an unqualified install detects the entire
catalog in catalog order. Tests will also confirm:

- `--yes` executes the complete eligible plan without interaction.
- The default interactive request supplies all tools preselected to the existing
  selection UI.
- `--profiles`, `--only`, and their combined form keep their current scope.
- An unknown explicit profile or tool remains an error before adapter mutation.

The complete existing Go and publication suites remain required.

## Documentation

The README and published site will present `jb tools install` as the primary
full-toolchain command, explain that all tools are preselected interactively,
and identify `jb tools install --yes` as the non-interactive form. Existing
profile and `--only` examples remain documented as scoped alternatives.

## Out of Scope

- Changing the contents or order of `tools.Catalog`.
- Adding a new “all” profile or `--all` flag.
- Changing `jb tools update` behavior.
- Removing interactive selection from the default command.
- Automatically implying `--yes` when no scope is supplied.

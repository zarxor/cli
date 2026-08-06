# Keyboard Selection and Color Design

## Goal

Replace the primary numbered tool prompt with a keyboard-driven multi-select and
apply a consistent semantic color system to every human-facing `jb` command.
The experience must remain portable across supported Windows and Linux terminals
and retain plain-text behavior for redirected input and output.

This work applies to the end-user `jb` binary. The maintainer-only release
command, bootstrap installer scripts, generated shell-completion data, and other
machine-readable output remain unstyled.

## Interactive Tool Selection

When both input and output are attached to a terminal, interactive install and
update commands show an inline multi-select. All eligible tools retain their
existing default-selected state and catalog order.

```text
Select tools to install

  ↑/↓ move   Space toggle   Enter continue   Esc cancel

❯ [✓] Git
  [✓] GitHub CLI
  [✗] Docker
  [✓] Docker Buildx
  [✓] Docker Compose
```

The controls are:

- Up and Down move the active row.
- Space toggles the active tool.
- Enter accepts the current selection and continues.
- Escape or Ctrl+C cancels before any host mutation.

The selected marker is `[✓]`, with only the check colored green. The deselected
marker is `[✗]`, with only the cross colored red. Brackets use the terminal's
neutral foreground. The active `❯` cursor uses the cyan accent, and the active
tool label is bold. Tool names otherwise use the terminal's normal high-contrast
foreground.

The selector returns selected tool IDs in their original input order regardless
of the order in which the user toggled them.

## Cancellation

Escape and Ctrl+C are intentional cancellation paths, not operational failures.
They restore the terminal state, print a muted `Cancelled — no changes made`
message, perform no installation or update, and exit successfully.

An unexpected terminal, rendering, or input error remains a failure and is
reported before any host mutation. Cursor visibility and terminal modes must be
restored on successful selection, cancellation, and failure.

## Portable Fallback

The existing numbered selector remains available when either input or output is
not a terminal. It preserves the current comma-separated number toggling and
empty-line acceptance behavior. The fallback contains no ANSI color sequences.

An adaptive selector chooses the keyboard UI only when both streams support it.
If the keyboard UI cannot start for a recoverable terminal-capability reason,
the command uses the numbered selector when its streams remain usable. Actual
I/O errors are returned rather than hidden.

`--yes` continues to bypass selection entirely. `--dry-run` remains
non-mutating, and all existing profile, explicit-tool, dependency, installation,
and update semantics remain unchanged.

## Architecture

The existing `SelectionUI` boundary remains the contract used by the install
service. Rendering is divided into focused components:

1. `InteractiveSelection` adapts a Huh multi-select to `SelectionUI`. It owns
   keyboard interaction, selection state, cancellation, and the inline view.
2. `NumberedSelection` remains the terminal-independent fallback.
3. `AdaptiveSelection` detects the streams and delegates to the correct
   selector without changing planner or installer behavior.
4. `Theme` defines semantic styles and symbols once for all human-facing `jb`
   rendering.
5. Focused render helpers apply the theme to selection, version tables, plans,
   results, errors, version output, and Cobra help.

Huh supplies the maintained cross-platform multi-select behavior. Lip Gloss
supplies terminal-aware styling and color-profile downsampling. These
dependencies are isolated behind the repository's rendering package so command,
planning, and installation code do not depend directly on terminal UI details.

## Semantic Theme

The theme uses restrained semantic color rather than decorating every token:

- Cyan: headings, the active cursor, command names, and important labels.
- Green: `[✓]`, successful installs and updates, verified tools, and up-to-date
  state.
- Red: `[✗]`, error prefixes, failures, and failed tools.
- Yellow: warnings, dry-run state, and work that has not completed.
- Muted gray: keyboard hints, secondary versions, skipped tools, and supporting
  explanations.
- Bold terminal foreground: important values that must stay readable on every
  background.

The palette adapts to light and dark terminal backgrounds and available color
depth. Exact true-color values may be downsampled by the terminal styling layer.
Meaning never depends on color alone: symbols, words, and layout communicate the
same state in monochrome output.

## Color Policy

Color is enabled only for human-facing output attached to a color-capable
terminal. It is disabled when:

- output is redirected or piped;
- the `NO_COLOR` environment variable is present, regardless of its value; or
- the command is producing shell completion or another machine-readable form.

Plain output must contain no ANSI escape sequences. The keyboard selector may
still run with colors disabled when both streams are terminals, using its symbols
and bold/neutral layout where supported.

## User-Facing Output

The theme covers all human-facing `jb` output:

- tool selection;
- install and update plans;
- dry-run results;
- successful, up-to-date, skipped, warning, and failed states;
- error presentation;
- version output; and
- Cobra help headings, command names, and flags.

Generated completion content stays byte-safe and unstyled. Styling is performed
at rendering boundaries rather than by wrapping stdout globally, preventing ANSI
sequences from leaking into data-oriented output.

## Error Handling

Selection completes before dependency ordering and host mutation, as it does
today. Cancellation returns a distinct result that the command translates into
the successful cancellation message. Other selector errors retain context and
flow through the existing error path.

Color detection failure degrades to plain rendering. It must never prevent a
tool operation. Write failures are still returned, and styling must not swallow
underlying writer errors.

## Testing

Unit and integration tests will cover:

- Up/Down navigation, Space toggling, Enter acceptance, and selected-ID order.
- Escape and Ctrl+C cancellation with no adapter mutation.
- Green `[✓]`, red `[✗]`, cyan cursor, and bold active labels in a controlled
  color profile.
- TTY selection versus automatic numbered fallback.
- Recoverable interactive-start fallback and unrecoverable I/O failure.
- Color-enabled output, `NO_COLOR`, redirected output, and monochrome output.
- No ANSI escapes in fallback, completion, or other machine-readable output.
- Semantic styling for plans, dry runs, successes, warnings, failures, version,
  and help.
- Terminal cleanup after success, cancellation, and error.
- Existing `--yes`, `--dry-run`, profile, `--only`, dependency, install, and
  update behavior.

Tests use injected streams, terminal capability decisions, color profiles, and
selection runners. They do not depend on a developer's actual terminal and do
not install or update host tools.

## Documentation

The README and public site will show the keyboard controls and selected/deselected
symbols. They will also state that redirected input retains the numbered prompt,
`--yes` remains the automation path, and `NO_COLOR` disables styling.

## Out of Scope

- Styling `cmd/release` or bootstrap installers.
- Changing the supported tool catalog, dependency rules, or default selection.
- Adding mouse controls, search, filtering, animation, or persistent UI
  preferences.
- Changing `--yes`, `--dry-run`, profiles, or `--only` semantics.
- Emitting color in completion scripts or machine-readable output.

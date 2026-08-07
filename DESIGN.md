---
name: Johan Bostrom CLI
description: A deliberate setup sheet for installing and updating a development toolchain.
colors:
  paper: "#f1f3eb"
  surface: "#fbfcf8"
  surface-strong: "#ffffff"
  ink: "#12161d"
  ink-soft: "#2f3742"
  muted: "#626b78"
  line: "#cbd2c9"
  line-strong: "#9fa9a2"
  blue: "#2d5bff"
  blue-dark: "#1738ad"
  coral: "#ff6858"
  coral-dark: "#a23d31"
  lime: "#d8f46d"
  night: "#151b23"
  night-panel: "#1d2631"
  night-line: "#3a4654"
  night-muted: "#aeb9c6"
  focus: "#f3b52f"
typography:
  display:
    fontFamily: "Bricolage Grotesque, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(3.2rem, 7.2vw, 6.4rem)"
    fontWeight: 700
    lineHeight: 1.06
    letterSpacing: "-0.055em"
  body:
    fontFamily: "Bricolage Grotesque, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "IBM Plex Mono, SFMono-Regular, Consolas, Liberation Mono, monospace"
    fontSize: "0.65rem"
    fontWeight: 600
    lineHeight: 1.55
    letterSpacing: "0.08em"
rounded:
  subtle: "0.18rem"
  panel: "0"
spacing:
  content: "3.5rem"
  section: "clamp(4.75rem, 9vw, 8rem)"
  tight: "0.75rem"
components:
  button-primary:
    backgroundColor: "{colors.blue}"
    textColor: "{colors.surface-strong}"
    rounded: "{rounded.subtle}"
    padding: "0.75rem 1rem"
    height: "2.9rem"
  code-block:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.surface-strong}"
    rounded: "{rounded.panel}"
    padding: "1rem 1.1rem"
---

# Johan Bostrom CLI design system

## Overview

**Creative North Star: “The Calibration Bench.”**

The site is a reversible machine setup sheet: calm enough to trust, precise enough to use while a terminal is open. It frames `jb` as an instrument for inspecting, installing, and updating a development environment—not as a vague productivity brand.

The visual language is matte paper, ink rules, electric-blue calibration marks, coral warnings, lime readiness signals, and monospaced measurements. The page should feel authored and operational. Every major section answers a practical question: what is managed, what will change, which platform applies, and what command runs next.

The system is intentionally static-site friendly. It keeps commands readable in the HTML, uses small progressive-enhancement behaviors for copying and tabs, and treats accessibility and reduced motion as part of the finish review.

## Colors

Use `paper` for the calibrated page field and `surface` for readable content surfaces. Use `ink` for primary text, rules, and high-confidence boundaries; use `ink-soft` and `muted` for supporting copy only.

`blue` is the action and navigation signal. `blue-dark` is reserved for readable link text and darker interactive states. `coral` marks warnings, active calibration accents, and the planned-platform boundary; use `coral-dark` wherever the accent is used as text. `lime` means ready, verified, or no-change state and should remain sparse.

The command section uses the `night` family as a deliberate tonal shift. `night-panel`, `night-line`, and `night-muted` preserve hierarchy without introducing a second visual brand. `focus` is reserved for keyboard focus rings and must remain visibly distinct from the content palette.

## Typography

Display and body text use Bricolage Grotesque with system fallbacks. Large headings are bold, tightly tracked, and allowed to wrap into short editorial lines. The blue emphasis in a heading is underlined with coral; do not use gradient text.

IBM Plex Mono is the data voice: commands, labels, platform metadata, status text, version-like values, and small measurement captions. Labels use uppercase, compact tracking, and a restrained size. Code remains legible before it becomes decorative; never reduce a command below a comfortable reading size just to preserve a single line.

The heading scale is fluid. The primary display range is `clamp(3.2rem, 7.2vw, 6.4rem)` with `1.06` line-height and `-0.055em` tracking. On narrow screens, the hero uses a tighter `clamp(2.95rem, 13vw, 4.15rem)` range so the instrument panel still enters the first viewport.

## Layout

The content rail is `min(100% - 3.5rem, 78rem)` on wide screens, tightening to `2.5rem` and then `1.5rem` gutters at the responsive breakpoints. Sections use `clamp(4.75rem, 9vw, 8rem)` vertical padding and keep a visible boundary between modes.

The desktop hero is a two-column promise-and-proof composition: the left side makes the case, the right side shows the live machine-state instrument. Install, command, and system sections reuse a wide content column plus a narrower explanatory rail. The command section reverses the contrast and uses a ledger beside a sticky preview panel.

At `68rem`, the hero gap compresses. At `56rem`, the header becomes two rows and major grids stack; section anchors reserve `8.5rem` for that sticky header. At `38rem`, controls become full-width where useful, metadata becomes a short vertical readout, and code/copy layouts stack without horizontal page overflow.

## Elevation & Depth

This is a flat-by-default system. Depth comes from tonal bands, 1px and 2px borders, a 3px active command rule, and the dark command surface—not from floating cards. The hero’s faint circular calibration rings are atmospheric geometry, not a container.

The only substantial shadow is the skip-link shadow, which helps that temporary accessibility control clear the page. Small signal rings may use a low-opacity halo. Do not add decorative drop shadows to panels, buttons, or ledger rows.

Motion is sparse and explanatory: buttons lift by 1px on hover, state transitions are 160ms, and the instrument panel gets one 1.4s coral scanline on entry. `prefers-reduced-motion: reduce` disables the expressive motion and smooth scrolling.

## Shapes

Shapes are square and structured. Primary buttons use the subtle `0.18rem` radius; large panels and command surfaces use square corners. Instrument and workbench boundaries use 2px ink rules, while internal dividers use 1px lines. Avoid pills, oversized rounded cards, and soft container silhouettes: the interface should read like a sheet, ledger, or test fixture.

The `jb` mark is a compact two-cell calibration block. It is the one logo-like shape on the page; other visual marks are simple rules, squares, dots, and text labels.

## Components

### Header and navigation

The sticky header carries the `jb` mark, product name, primary anchors, and the blue `Install jb` action. Navigation uses mono-adjacent sizing and an underline active state rather than filled pills. On mobile it becomes a two-row instrument label: the brand/action row stays compact and the anchors get their own ruled row.

### Buttons and links

The primary button is blue with white text. The quiet button is transparent with a strong line and becomes a white surface on hover. Both use the subtle radius, a 1px boundary, a 160ms transition, and a 3px focus ring with 4px offset. Links are blue-dark, underlined, and visibly darker or brighter on hover.

### Instrument profile panel

The hero proof object is a single bordered panel, not a card collection. Its dark mono header announces live machine state; the white manifest lists the development profile; the blue command band exposes the exact dry-run command; the lime footer states that no changes were made. One coral scanline supplies the only cinematic moment.

### Platform workbench

The install section uses a ruled tab strip and one visible platform panel. Tabs have real `role=tab` semantics, expose their selected state, support left/right keyboard navigation, and keep the command plus review path in plain text. Linux is the default view; Windows switches to the literal PowerShell review command `.\install.ps1` as rendered by the page.

### Command ledger and preview

Commands are rows separated by rules, with one featured `jb tools install` row marked by a blue top rule. Each row says what it does and exposes a copyable inline command. The adjacent preview panel keeps the dry-run output visible as evidence. This is a ledger, not a grid of repeated feature cards.

### Code blocks and copy controls

Code blocks use the ink surface, mono text, horizontal scrolling only inside the block, and a compact copy button separated by a rule. Copying updates a live status label and has a clipboard fallback. Commands must remain understandable if JavaScript is unavailable.

## Do's and Don'ts

### Do

- Keep the matte paper, ink, blue, coral, and lime roles distinct.
- Use mono labels for operational data and editorial display type for promises.
- Show the command, the expected change boundary, and the review path together.
- Preserve the calibration grid, ruled ledgers, and single instrument-panel scanline as the page’s visual grammar.
- Maintain visible keyboard focus, semantic landmarks, real tab semantics, and reduced-motion behavior.

### Don't

- Do not reintroduce generic rounded card grids, glass effects, gradient text, or hard offset shadows.
- Do not use pills or emoji as the primary navigation, status, or platform language.
- Do not hide install/update commands behind JavaScript or make unverifiable support claims.
- Do not turn every accent into a badge; blue is action, coral is caution, and lime is readiness.
- Do not add motion that competes with the command surface or survives reduced-motion preferences.

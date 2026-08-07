# Product

<!-- impeccable:product-schema 1 -->

> The following product facts are inferred from the existing repository, README, installer scripts, and published page. The visual brief delegated design direction to Codex; no product claims are being added.

## Platform

web

## Users

Developers setting up or refreshing a development machine, plus maintainers who need a clear reference for installing and updating `jb`.

## Product Purpose

Johan Bostrom CLI (`jb`) installs and updates a curated development toolchain from a standalone binary. The site helps a visitor understand what `jb` does, install it safely, and find the most useful commands quickly.

## Positioning

`jb` plans from live machine state, previews changes, uses native providers, verifies release checksums, and keeps privilege boundaries explicit without a local state database or telemetry.

## Operating Context

Visitors arrive from a shell or repository context, choose a platform, copy a one-line installer or review the bootstrap script, then use `jb tools install`, `jb tools update`, or `jb update` in a terminal.

## Capabilities and Constraints

- The site is plain static HTML, CSS, and JavaScript published through GitHub Pages at `cli.johanbostrom.se`.
- It must keep commands, supported platforms, installer URLs, and release behavior accurate.
- Linux support covers Debian/Ubuntu and Arch Linux; Windows support uses PowerShell.
- macOS support is planned, not currently available.
- Copy buttons must keep commands available as selectable plain text and remain keyboard accessible.

## Brand Commitments

- Product name: Johan Bostrom CLI.
- CLI name: `jb`.
- Existing repository and release links remain valid.

## Evidence on Hand

- `README.md` contains the canonical install, update, platform, and publishing details.
- `site/index.html`, `site/styles.css`, and `site/site.js` are the current published surface.
- `install.sh`, `install.ps1`, and `CNAME` are deployed beside the page.

## Product Principles

- Show the next useful command without hiding the reasoning behind it.
- Make preview, verification, and privilege boundaries visible.
- Keep the happy path fast while leaving review paths available.
- Prefer truthful scope over inflated claims.

## Accessibility & Inclusion

Preserve semantic landmarks, keyboard navigation, visible focus, readable contrast, reduced-motion support, responsive layouts, and copy feedback announced to assistive technology.

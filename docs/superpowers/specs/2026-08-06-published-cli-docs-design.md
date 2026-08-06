# Published Johan Bostrom CLI Documentation Design

## Goal

Give `https://cli.johanbostrom.se/` a user-facing static documentation page
that explains how to install and use `jb`, while keeping internal planning
documents out of the published site.

## Source layout

The repository will contain a dedicated `site/` directory. Its `index.html`
will be the single user-facing documentation page and will use local CSS so
the site remains self-contained and works without a build tool or external
runtime.

The page will cover:

- what Johan Bostrom CLI is and which operating systems it supports;
- Linux and Windows bootstrap installation links;
- the `jb tools install` command, development profiles, `--only`, `--yes`, and
  `--dry-run`;
- the `jb tools update` command and live, stateless detection;
- the development tool catalog and platform/elevation behavior;
- links to the repository and release assets.

The root `install.sh`, `install.ps1`, and `CNAME` remain the maintained source
files. They will be copied into the generated Pages artifact, avoiding a
second installer implementation in `site/`.

## Publishing

The Pages workflow will build a temporary `_site` directory by copying the
contents of `site/` and then copying the maintained root installers and
`CNAME` into that directory. The workflow will upload `_site`, not the entire
repository. This prevents `docs/superpowers` and other source files from being
published while preserving the existing installer URLs.

## Verification

Publication tests will verify that:

- `site/index.html` exists and includes the product name, command examples,
  supported platforms, and both installer URLs;
- the Pages workflow publishes `_site` and copies the maintained installers;
- the site has no references to the retired installer URL;
- existing Bash, Go, installer, and artifact checks remain green.


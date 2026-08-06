# Published CLI Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a user-facing static documentation site under `site/` and publish it at `cli.johanbostrom.se` without exposing internal repository documents.

**Architecture:** `site/index.html` and `site/styles.css` are a self-contained static site with no build tool or external runtime. The Pages workflow assembles a temporary `_site` directory from `site/`, the maintained root installers, and `CNAME`, then uploads `_site` to GitHub Pages. Publication tests validate both the source site and the assembly contract.

**Tech Stack:** HTML5, CSS, Bash publication tests, GitHub Actions Pages artifact upload.

## Global Constraints

- The published hostname remains `https://cli.johanbostrom.se/`.
- The public site source lives only under `site/`; internal `docs/superpowers` files are not uploaded.
- The maintained root `install.sh`, `install.ps1`, and `CNAME` remain the single source files copied into the Pages artifact.
- The site must work as static files without JavaScript, a package manager, external fonts, or a framework.
- Existing Go, Bash, installer, and artifact validation must remain green.
- The old `scripts.johanbostrom.se` and `linux/dev-server/setup.sh` URLs must not be reintroduced.

---

## File map

- Create: `site/index.html` — user-facing documentation page and command reference.
- Create: `site/styles.css` — local responsive styling for the page.
- Modify: `.github/workflows/pages.yml` — assemble `_site` from `site/`, installers, and `CNAME`, then upload `_site`.
- Modify: `tests/publication.bash` — validate site source, workflow assembly, installer links, and retired URL absence.
- Modify: `README.md` — identify `site/` as the Pages source.

### Task 1: Add the static documentation page

**Files:**

- Create: `site/index.html`
- Create: `site/styles.css`
- Test: `tests/publication.bash`

**Interfaces:**

- `site/index.html` references `styles.css` relatively and links to `/install.sh`, `/install.ps1`, the repository, and Releases.
- The page includes the exact commands `jb tools install --profiles=development`, `jb tools install --profiles=development --only=bun`, `jb tools update`, and `jb tools update --profiles=development --only=bun`.

- [ ] **Step 1: Write the failing publication assertions**

Add assertions requiring `site/index.html`, `site/styles.css`, the product name, both installer URLs, supported platform names, all four command examples, and no retired URL text.

- [ ] **Step 2: Run the test to verify it fails**

Run `bash tests/publication.bash`. Expected: FAIL because the site files do not exist.

- [ ] **Step 3: Create the static page and stylesheet**

Create semantic HTML with a product header, Linux/Windows install cards, install/update/profile command sections, tool catalog, platform notes, repository/Releases links, and a static-site footer. Keep all styling in `site/styles.css`, use system fonts, visible focus states, responsive layout, and no JavaScript.

- [ ] **Step 4: Run the test to verify it passes**

Run `bash tests/publication.bash`. Expected: PASS for the new and existing assertions.

- [ ] **Step 5: Commit**

```bash
git add site/index.html site/styles.css tests/publication.bash
git commit -m "docs: add published CLI site"
```

### Task 2: Publish a clean Pages artifact

**Files:**

- Modify: `.github/workflows/pages.yml`
- Modify: `tests/publication.bash`

**Interfaces:**

- The Pages job creates `_site`, copies `site/.`, then copies `install.sh`, `install.ps1`, and `CNAME` into `_site` before uploading `path: _site`.
- Existing Go and Bash validation jobs remain unchanged except for necessary path assertions.

- [ ] **Step 1: Write failing workflow assertions**

Require `_site` assembly, `site/.`, both installers, `CNAME`, and `path: _site`; reject `path: .`.

- [ ] **Step 2: Run the test to verify it fails**

Run `bash tests/publication.bash`. Expected: FAIL because the workflow uploads the repository root.

- [ ] **Step 3: Update the Pages workflow**

Add this deploy step before upload:

```yaml
      - name: Assemble Pages site
        run: |
          rm -rf _site
          mkdir -p _site
          cp -R site/. _site/
          cp install.sh install.ps1 CNAME _site/
```

Set the upload path to `_site` and do not copy `docs/` or other source directories.

- [ ] **Step 4: Run the test to verify it passes**

Run `bash tests/publication.bash`. Expected: PASS, including workflow assertions.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/pages.yml tests/publication.bash
git commit -m "build: publish dedicated Pages site"
```

### Task 3: Document the source and run complete verification

**Files:**

- Modify: `README.md`

- [ ] **Step 1: Update README**

Add a sentence in Publishing stating that `site/` is the user-facing Pages source and internal planning documents are excluded from deployment.

- [ ] **Step 2: Run all checks**

```bash
go test ./... -count=1
go vet ./...
go build ./cmd/jb
bash -n install.sh tests/cli-smoke.sh tests/*.bash
bash tests/run.bash
git diff --check
```

Expected: all commands exit 0 and publication tests include the new site and assembly assertions.

- [ ] **Step 3: Inspect the site locally**

Run `python3 -m http.server 8080 --directory site`, open `http://127.0.0.1:8080/`, confirm the page and links render at desktop and narrow widths, then stop the server.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: explain Pages source"
```

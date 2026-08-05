# Johan Boström's scripts

Reusable scripts published through GitHub Pages at
[`scripts.johanbostrom.se`](https://scripts.johanbostrom.se).

Every public script is documented here with its permanent URL, supported
systems, effects, privileges, rerun behavior, and verification steps.

## Linux development machine

[`linux/dev-server/setup.sh`](https://scripts.johanbostrom.se/linux/dev-server/setup.sh)
installs or updates a development toolchain on Debian-family and Arch-family
Linux systems.

It installs the latest stable versions available from supported sources:

- Codex CLI
- Git
- GitHub CLI (`gh`)
- Docker Engine, Buildx, and Docker Compose
- nvm and the latest Node.js LTS
- the latest npm
- pnpm and Yarn through Corepack
- Bun

### Requirements

- Debian, Ubuntu, a compatible Debian-family distribution, Arch Linux, or a
  compatible Arch-family distribution
- Bash 4.4 or newer
- A non-root user with working `sudo` access
- An internet connection

Do not run the script as root. It requests `sudo` only for system packages,
repository configuration, the Docker service, and an optional group change.
User tools are installed into the invoking user's home directory.

### Run it

The recommended command keeps standard input connected to the setup wizard:

```bash
bash <(curl -fsSL https://scripts.johanbostrom.se/linux/dev-server/setup.sh)
```

To review the script before running it:

```bash
curl -fsSLO https://scripts.johanbostrom.se/linux/dev-server/setup.sh
less setup.sh
bash setup.sh
```

Avoid `curl ... | bash`: the pipe consumes standard input that the optional
wizard needs.

### First run and updates

The script detects the distribution, installs missing tools, updates installed
tools through their stable supported sources, verifies every tool, and then
offers the wizard. You can safely rerun the same command later; package sources
and shell profile blocks are replaced or reused instead of duplicated.

Arch uses a full `pacman -Syu` transaction to avoid unsupported partial
upgrades. Debian-family systems use apt and supported GitHub CLI and Docker
repositories. A Debian derivative must declare a compatible Debian or Ubuntu
codename that exists in Docker's repository.

### Optional setup wizard

The wizard asks separately whether to:

1. Set or replace the global Git name and email.
2. Authenticate GitHub CLI with `gh auth login`.
3. Authenticate Codex with `codex login`.
4. Add the current user to the `docker` group.
5. Make the latest Node.js LTS the nvm default.

Existing Git identity and authentication are preserved unless you explicitly
choose to change them. Membership in the docker group grants root-level privileges.
The script displays that warning and requires confirmation before
changing group membership. Log out and back in afterward for the group change
to take effect.

### Verify the installation

Open a new shell and run:

```bash
git --version
gh --version
docker --version
docker compose version
nvm --version
node --version
npm --version
pnpm --version
yarn --version
bun --version
codex --version
```

If nvm, Bun, or Codex is not found immediately, open a new shell so the updated
`.bashrc` is loaded. If Docker still requires `sudo` after accepting its wizard
step, log out and back in. Authentication can be completed later with
`gh auth login` and `codex login`.

## Publishing and custom domain

GitHub Pages must use **GitHub Actions** as its source. The deployment workflow
publishes `main` only after syntax, ShellCheck, behavioral, and publication
checks pass.

For the custom domain:

1. Verify `scripts.johanbostrom.se` in the owning GitHub account's Pages domain
   settings.
2. Create a DNS CNAME record named `scripts` pointing to `zarxor.github.io`.
3. Wait for GitHub's DNS check, then enable HTTPS in the repository's Pages
   settings.

The repository's `CNAME` file declares the same hostname.

## Contributing

Keep public URLs stable and scripts safe to rerun. Any change that adds,
renames, or changes a public script must update this README in the same commit.
Run the local checks with:

```bash
bash tests/run.bash
shellcheck --severity=warning linux/dev-server/setup.sh tests/*.bash
```

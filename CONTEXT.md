# Johan Bostrom CLI Context

This context defines the product language for the local development toolchain and its Agent Skill extensions.

## Toolchain

**Tool**:
An executable developer utility, runtime, provider, or command-line client that `jb` can install or update.
_Avoid_: App, package

**Toolchain**:
The coherent set of tools needed for a development workflow.
_Avoid_: App bundle, software collection

**Provider**:
The installation mechanism that owns a tool on a particular host, such as a system package manager, runtime manager, or user-level package path.
_Avoid_: Source, vendor

**Component**:
An executable or supporting capability that belongs to a larger tool, such as a Docker plugin or Node package manager.
_Avoid_: Separate app

**Profile**:
A built-in named set of tools that `jb` applies before planning an install or update. Profiles are not persisted.
_Avoid_: Preset, bundle

**Automatic profile**:
The profile selected from the detected host role when the user does not provide
`--profiles` or `--only`. Current automatic profiles are `desktop` and `server`.
_Avoid_: Guess, default package list

**Host role**:
The best-effort classification of the current machine as `desktop` or `server`,
with a human-readable detection reason shown to the user.
_Avoid_: Platform

## Agent capabilities

**Agent Skill**:
A portable capability bundle that an AI agent can load for a specialized task, centered on a `SKILL.md` document and optional supporting resources.
_Avoid_: Prompt, AI app, tool

**Plugin**:
A broader package that may combine Agent Skills with applications, integrations, templates, or other workflow capabilities.
_Avoid_: Skill

**Instruction**:
Persistent guidance that applies broadly to a project or agent, rather than a task-specific capability bundle.
_Avoid_: Skill

**Target**:
The AI agent or client that discovers and uses an Agent Skill.
_Avoid_: Provider, platform

**Harness**:
A user-selected Agent Skill destination, currently Codex or Claude. Harness
selection determines which exact target locations are checked and written by
an install run.
_Avoid_: Unknown AI integration

**Scope**:
The ownership boundary of an Agent Skill: global (user-wide) or specific to
the current project. An install run chooses one scope before the skill list is
shown.
_Avoid_: Target

**Scope choice**:
The installation decision made before skill selection: global (user-wide) or
the current project.
_Avoid_: Target

**Skill catalog**:
The explicit product-owned list of Agent Skills that `jb` may present for
installation or update.
_Avoid_: Registry, marketplace

**Available skill**:
An Agent Skill represented by an entry in the skill catalog, whether or not it
is installed on the current host.
_Avoid_: Recommended skill

**Skill creator**:
The person or organization responsible for a catalogued Agent Skill collection;
the install selector uses it as the grouping boundary.
_Avoid_: Provider

**Skill selection**:
The user's chosen subset of available skills for one install or update run.
_Avoid_: Auto-install set

**Skill source**:
The catalog-owned local directory, repository reference, or archive from which
an Agent Skill is obtained. It is not a command-line argument.
_Avoid_: Provider

**Managed skill**:
An Agent Skill whose source and content digest are recorded by `jb`, allowing safe verification, update, and removal.
_Avoid_: Installed app

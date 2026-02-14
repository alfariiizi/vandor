# Contributing to Vandor CLI

Thank you for contributing.

This repository contains Vandor CLI core.

## Scope of This Repo

Use this repo for:

1. command surface (`new`, `add`, `sync`, `vpkg`, `run`, `dev`, `tui`)
2. core scaffolding and sync behavior
3. config and runtime contracts
4. CLI integration tests

Do not embed opinionated transport/infrastructure implementations directly in core.
Use `vpkg` ecosystem for that.

## Prerequisites

1. Go (version from `go.mod`)
2. Task
3. Git

## Local Setup

```bash
task setup
task check
task test
```

## Branching and Commits

1. Create focused feature branches.
2. Keep commits small and reviewable.
3. Use clear commit messages.

## Required Checks Before PR

```bash
task check
task test
```

If behavior changes code generation:

1. run sync/generation tasks
2. include generated output changes in PR

## Design Constraints

1. Default CLI mode must be non-interactive.
2. Interactive prompts only with `-it`/`--interactive`.
3. TUI is UX layer; CLI remains source of truth.
4. Generated outputs must be deterministic.
5. Runtime should not require installing `vandor` binary.

## Documentation Requirements

When command/architecture behavior changes, update workspace specs in `../codex-docs/`:

1. `spec-vandor-core-boundary-v1.md`
2. `spec-context-scaffold-v1.md`
3. `spec-cli-interaction-v1.md`
4. `spec-vpkg-v1.md`
5. `roadmap-vandor-v0.4.md`

## Pull Request Guidelines

Please include:

1. problem statement
2. scope of change
3. testing evidence
4. migration impact (if any)

## Acknowledgment

Vandor CLI development direction was initially accelerated by learning from [peiman/ckeletin-go](https://github.com/peiman/ckeletin-go). Thank you for the baseline ideas and tooling discipline.

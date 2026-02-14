# Vandor CLI (v0.4 Direction)

Vandor is a Golang CLI for building backend projects with:

1. domain-first design (DDD)
2. practical hexagonal architecture
3. community-driven transport and infrastructure packages (`vpkg`)

This repository is the CLI core only.

## Project Status

This codebase is under active refactoring toward Vandor v0.4.

Current direction:

1. Core-first scaffold (`vandor new` default is neutral/core-only).
2. Fast scaffolding for bounded contexts (`vandor add context|domain|usecase|service`).
3. Deterministic sync (`vandor sync ...`).
4. Package ecosystem (`vandor vpkg ...`) for transport/infrastructure setup.
5. Dual command mode:
- default non-interactive (AI/CI safe)
- explicit interactive mode with `-it` / `--interactive`

## Why Vandor

Vandor is designed for three realities:

1. one-man-army speed
2. team collaboration with strict context boundaries
3. long-term transition from modular monolith to microservices

## Command Surface (Planned/Core)

1. `vandor new <project> [--module ...]`
2. `vandor add context <name>`
3. `vandor add domain <context>`
4. `vandor add usecase <context> <name>`
5. `vandor add service <context> <name>`
6. `vandor sync core|context|all`
7. `vandor vpkg add|remove|list|sync|exec|doctor`
8. `vandor run:app|worker|scheduler|all`
9. `vandor dev:app|worker|scheduler|all`
10. `vandor tui`

## Architecture Boundary

Vandor core manages:

1. `internal/core/contexts/*` (domain + application scaffolding)
2. code generation and sync contracts
3. config lifecycle and runtime wiring contracts

Vandor core does not hard-code transport/infrastructure framework choices.
Those are delivered through `vpkg` packages.

## Repository Topology

Vandor ecosystem uses polyrepo:

1. `vandor` (this repo): CLI core
2. `vpkg`: package registry and package sources
3. `vandor-apps`: docs and public apps

## Development Workflow

```bash
task setup
task check
task test
task build
```

## Source of Truth for Design

Workspace architecture specs are maintained under:

- `../codex-docs/`

Important specs:

1. `spec-vandor-core-boundary-v1.md`
2. `spec-vpkg-v1.md`
3. `spec-context-scaffold-v1.md`
4. `spec-cli-interaction-v1.md`
5. `spec-repository-strategy-v1.md`

## Acknowledgment

Thank you to [peiman/ckeletin-go](https://github.com/peiman/ckeletin-go) for being the baseline that inspired the initial CLI management structure used to evolve Vandor.

## License

MIT

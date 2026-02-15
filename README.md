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

## Command Surface

Phase-1 available now:

1. `vandor new <project> [--module ...]`
   - supports `--tidy auto|always|never` (default: `auto`)
2. `vandor add context <name>`
   - supports `--tidy auto|always|never` (default: `auto`)
3. `vandor add domain <context> <name>`
4. `vandor add valueobject <context> <name> [--kind ...] [--enum ...]`
5. `vandor add usecase <context> <name>`
6. `vandor add service <context> <name>`
7. `vandor sync core|context|all`
8. `vandor vpkg add|remove|list|search|sync|doctor|exec|exec-alias`
9. `vandor vpkg registry add|list|remove`
10. `vandor run:app|run:worker`
11. `vandor dev:app|dev:worker`
12. `vandor tui` (Phase-1 placeholder, TTY-only)

Notes:

1. `vandor add ...` commands auto-run sync wiring for affected context + core.
2. `vandor sync ...` remains available for explicit/manual resync.
3. Current `vpkg` source support is local-first:
   - local path (`./path/to/package`)
   - git source (`github.com/org/repo//packages/pkg@ref` or local git repo path with `//subdir`)
   - registry alias backed by local/file/http registry URLs in `vpkg.yaml`
   - bare alias fallback (for example `http-humachi` or `official/http-humachi`)
   - registry HTTP index resolution baseline
4. When `vpkg.yaml` does not exist yet, default registries are discovered in order:
   - `official`
   - `verified`
   - `community`
   (with env override support via `VANDOR_VPKG_REGISTRY_OFFICIAL|VERIFIED|COMMUNITY`)
5. `vandor vpkg add --plan` provides dry-run install plan without writing files.
6. `vandor vpkg add` auto-installs dependency chain from package manifest.
7. In interactive mode (`-it`), `vpkg add` asks confirmation before applying write operations.
8. `vandor vpkg doctor --fix` attempts safe repair by re-syncing installed packages from cache/lock.
9. `vandor vpkg remove` now blocks removing a package that is still required by others, unless `--force` is used.
10. Alias shortcut is supported: unknown command with `:` will fallback to installed vpkg alias (for example `vandor add:http-handler`).
11. `vandor vpkg search` supports pagination with `--limit` (default `10`) and `--offset`.

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

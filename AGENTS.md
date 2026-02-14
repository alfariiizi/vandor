# AGENTS Guide (vandor CLI Repo)

This repository contains Vandor CLI core.

## Scope

Implement and maintain:

1. command surface (`new`, `add`, `sync`, `vpkg`, `run`, `dev`, `tui`).
2. domain/application scaffolding contracts.
3. code generation and sync determinism (`*_gen.go`).
4. config loading/validation and runtime bootstrap contracts.

Do not hard-code transport/infrastructure frameworks into core.  
Transport and infrastructure setup must be delivered through `vpkg`.

## Architecture Rules

1. Keep core generation neutral by default (`vandor new` produces core-only scaffold).
2. Keep command behavior non-interactive by default.
3. Enable prompts only with explicit `-it` or `--interactive`.
4. Keep TUI as UX layer over CLI source-of-truth commands.

## Testing Rules

1. Add/update unit tests for command parsing and generation logic.
2. Add integration tests for `new/add/sync/vpkg` flows where behavior spans files.
3. Ensure deterministic output order for generated artifacts.

## Documentation Sync

When changing behavior, update relevant files in workspace `codex-docs/`:

1. `spec-vandor-core-boundary-v1.md`
2. `spec-context-scaffold-v1.md`
3. `spec-cli-interaction-v1.md`
4. `spec-vpkg-v1.md`
5. `roadmap-vandor-v0.4.md`


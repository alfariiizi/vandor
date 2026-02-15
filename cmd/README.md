# cmd Package

This package defines Vandor CLI command surface for Phase-1.

## Commands

1. `new`
2. `add context|domain|valueobject|usecase|service` (`valueobject` supports `--kind` and `--enum`)
3. `sync core|context|all`
4. `vpkg add|remove|list|search|sync|info|doctor|exec|exec-alias`
5. `vpkg registry add|list|remove`
6. `dev:app|dev:worker`
7. `run:app|run:worker`
8. `tui` (TTY-only placeholder)
9. `completion`

## Interaction Model

1. Non-interactive by default.
2. Interactive mode is explicit via `--interactive` or `-i`.
3. `-it` pattern is supported (`-i` + hidden `-t` alias).

## Notes

1. Core scaffolding logic lives in `internal/coregen`.
2. Command output can be `--output text|json`.
3. `add` commands auto-sync affected context + core wiring.
4. `vpkg add --plan` prints dry-run plan without applying writes.
5. `vpkg add` resolves and installs missing dependencies first.
6. `vpkg add` accepts registry aliases with and without `@` prefix.
7. Default registry tier order is `official -> verified -> community` when `vpkg.yaml` is missing.
8. Interactive confirmation for write operations is enabled when `-it` is used.
9. `vpkg doctor --fix` re-syncs installed package files from lock cache to repair drift.
10. `vpkg remove` protects dependency graph by default; use `--force` to bypass.
11. Root-level alias fallback is enabled for `:` commands (example: `vandor add:http-handler`).
12. `vpkg search` supports `--registry`, `--limit` (default `10`), and `--offset` pagination flags.
13. `vpkg doctor` text output includes machine-friendly issue codes.
14. `vpkg remove --force` prints an impact preview before removal.
15. `vpkg info <package-or-source>` prints package usage (actions, aliases, install hint, and README path when available).

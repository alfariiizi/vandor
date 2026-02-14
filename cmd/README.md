# cmd Package

This package defines Vandor CLI command surface for Phase-1.

## Commands

1. `new`
2. `add context|domain|valueobject|usecase|service` (`valueobject` supports `--kind` and `--enum`)
3. `sync core|context|all`
4. `vpkg add|remove|list|sync|doctor` (placeholder)
5. `dev:app|dev:worker`
6. `run:app|run:worker`
7. `tui` (TTY-only placeholder)
8. `completion`

## Interaction Model

1. Non-interactive by default.
2. Interactive mode is explicit via `--interactive` or `-i`.
3. `-it` pattern is supported (`-i` + hidden `-t` alias).

## Notes

1. Core scaffolding logic lives in `internal/coregen`.
2. Command output can be `--output text|json`.
3. `add` commands auto-sync affected context + core wiring.

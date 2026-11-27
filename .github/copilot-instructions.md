# Copilot Instructions for scw-tui

## Overview

A keyboard-driven terminal UI (TUI) for managing Scaleway cloud resources, written in Go with [Bubbletea](https://github.com/charmbracelet/bubbletea) and the Dracula colour theme.

**Services supported:** Instances (servers + power actions), Volumes & Snapshots, Object Storage (S3), Kubernetes (Kapsule), Container Registry, Secrets Manager, Billing, and a read-only IAM browser.

**[CLAUDE.md](../CLAUDE.md) is the authoritative architecture document** — read it before making changes. This file only summarises the essentials.

## Developer Workflows

```bash
make build        # go mod tidy + go build -o bin/scw-tui main.go
make test         # go vet ./... + go test .
make lint         # golangci-lint + gosec + govulncheck (run `make install-tools` once)
./bin/scw-tui     # run
```

CI (`.github/workflows/ci.yml`) runs a `lint` job (go vet + golangci-lint) and a `test` job (go test), together equivalent to `make test`; `security.yml` adds gosec + govulncheck.

## Architecture

- **Single `package main`**, split across focused files: `main.go` (state, palette, Init/Update/View), `types.go`, `config.go`, `keys.go`, `filter.go`, `util.go`, plus one `cmd_*.go` (async API ops) and one `view_*.go` (rendering) per service.
- **UI state** lives entirely in the `rootModel` struct — there is no other state manager.
- **Navigation** is a two-axis state machine: `state` constants select the screen (profile picker, dashboard, object/K8s/registry/secrets/IAM/billing browsers), `service` constants select which dashboard service is active. Instances and Volumes are dashboard-level only (no dedicated `state`).
- **Profiles/credentials** come from the Scaleway SDK config (`~/.config/scw/config.yaml`) via `scw.LoadConfig()`; only UI preferences (active profile + active region) live in `~/.config/scw-tui/config.json`. Never duplicate credentials.

## Patterns & Conventions

- **Async I/O:** all blocking work (API calls, uploads) is a `tea.Cmd` closure returning a typed message (e.g. `serversMsg`, `volumesMsg`, `errMsg`). Errors are wrapped in `errMsg` — never returned bare.
- **Background goroutines** (upload progress only) may use the global `teaProgram.Send()`; prefer `tea.Cmd` for everything else.
- **Overlays** are boolean flags on `rootModel` rendered in `View()` (input dialog, confirm dialogs, progress bars, pickers).
- **Keyboard:** vim-style (`j`/`k`, `/` filter, `Enter`, `Esc` back, `q` quit) plus per-service single-key actions dispatched centrally in `handleKey()` (`keys.go`). Per-screen key tables live in [README.md](../README.md).
- **S3 ops** go through minio-go; **all other Scaleway resources** through scaleway-sdk-go.
- **Styling:** Dracula palette constants at the top of `main.go`. Do not introduce new colours without explicit user approval. Selected/highlighted list rows must be plain (uncolored) text so the row background spans the full width — see the styling notes in CLAUDE.md.
- **Polling:** lists refresh on a 3s tick while items are in transient states (server starting/stopping, volume snapshotting/attaching, …).

## Examples

**Adding a new service:** add a `service*` constant (`main.go`), extend `rootModel`, add a dashboard nav entry + preview pane (`view_dashboard.go`), create `cmd_<svc>.go` (fetch ops as `tea.Cmd`s) and `view_<svc>.go` (rendering), wire keys in `keys.go`, and document keys in README.md.

**Adding a key action:** route it in `handleKey()` in `keys.go` (input overlay and overlays intercept first) and implement the handler in the owning service's `keys_<service>.go`, then add a row to the README key table.

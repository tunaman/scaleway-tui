# Copilot Instructions for scw-tui

## Overview
This project is a terminal UI (TUI) for managing Scaleway resources, written in Go. It uses Bubbletea for UI, MinIO for S3-compatible object storage, and Scaleway's SDK for API access. The main entry point is `main.go`.

## Architecture
- **Single-file app:** All core logic is in `main.go`.
- **UI State:** Managed via a `rootModel` struct, with Bubbletea's update/view pattern.
- **Profiles:** Uses Scaleway SDK config (`~/.config/scw/config.yaml`) for credentials. UI preferences are stored in `~/.config/scw-tui/config.json`.
- **Services:** Supports Object Storage (S3) and Kubernetes (K8s) via service selection.
- **Data Flow:** Profile selection → client creation → dashboard → object browser or K8s cluster view.

## Developer Workflows
- **Build:**
  - `make build` (uses `Makefile`)
  - Output binary: `bin/scw-tui`
- **Clean:**
  - `make clean`
- **Docker:**
  - Build with Dockerfile: `docker build -t scw-tui .`
  - Runs Alpine, entrypoint is the built binary
- **Run:**
  - `bin/scw-tui` (after build)
- **Dependencies:**
  - Managed via `go.mod` (run `go mod tidy` before build)

## Patterns & Conventions
- **UI overlays:** Input, confirmation, and upload overlays are managed via state flags in `rootModel`.
- **Keyboard navigation:** Vim-like keys (`j/k`, `up/down`, `/` for filter, `c` for create, `u` for upload, `d` for delete, `a` for select all, `esc` for back/cancel, `q` for quit).
- **Bubbletea:** Uses `tea.Cmd` for async actions (fetch, upload, delete, etc.).
- **Error handling:** Errors are sent as messages (`errMsg`) and update UI state.
- **Styling:** Uses Dracula palette via Lipgloss.

## Integration Points
- **Scaleway SDK:** For API access, profile loading, and project/cluster info.
- **MinIO:** For S3-compatible object storage operations.
- **Bubbletea/Bubbles/Lipgloss:** For TUI rendering and interaction.

## Key Files
- `main.go`: All application logic, UI, and state
- `Makefile`: Build/clean commands
- `Dockerfile`: Container build
- `go.mod`: Dependency management

## Example: Adding a New Service
- Add a new constant to the `service*` enum
- Extend `rootModel` and update navigation logic
- Implement fetch/view logic for the new service

## Example: Custom Keyboard Action
- Add handling in `handleKey()` in `main.go`
- Update UI state as needed

---
**Feedback:** Please review and suggest edits for unclear or missing sections. This guide is meant to help AI agents quickly understand and contribute to scw-tui.
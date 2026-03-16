# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make build          # go mod tidy + go build -o bin/scw-tui main.go
make clean          # remove bin/

# Lint (matches CI)
go vet ./...
golangci-lint run

# Run
./bin/scw-tui
```

There are no automated tests in this project.

## Architecture

The codebase is a single Go package (`package main`) split across these files:

| File | Contents |
|---|---|
| `main.go` | Constants, Dracula palette, `rootModel` struct, `Init`, `Update`, `View`, `main` |
| `types.go` | All data structs and tea message types |
| `config.go` | `tuiConfig`, `loadTUIConfig`, `saveTUIConfig`, `buildClients` |
| `keys.go` | `handleKey`, `handleEsc`, `handleUp`, `handleDown`, `handleEnter`, `activateProfile` |
| `filter.go` | `filteredBuckets/RegistryImages/RegistryTags/RegistryNamespaces`, `maybeCalculateSize` |
| `util.go` | `panelBox`, `padRight`, `renderVScrollBar`, `formatBytes`, `formatEuroShort`, `parentPrefix`, `prevMonth`, `nextMonth`, `moneyToFloat`, `min`, `max` |
| `cmd_s3.go` | `fetchData`, `calculateSize`, `fetchBucketContents`, `createBucket/Folder`, `uploadFile`, `deleteEntries`, `progressReader` |
| `cmd_registry.go` | `fetchRegistryImages/Tags`, `deleteRegistryTags` |
| `cmd_billing.go` | `fetchBillingOverview`, `fetchConsumptionDetail`, `exportBillingCSV` |
| `view_picker.go` | `drawProfilePicker` |
| `view_dashboard.go` | `drawDashboard`, `renderTopBar/StatusBar/Nav/Content`, `renderBuckets/Clusters/Registry/BillingPreview` |
| `view_s3.go` | `drawObjectBrowser`, browser render helpers, `renderConfirmDialog/InputOverlay/UploadProgress` |
| `view_registry.go` | `drawRegistryBrowser`, `renderRegistryVersionPane`, tag action/delete overlays |
| `view_billing.go` | `drawBilling`, `renderBillingChart/Detail/TopBar/StatusBar` |

### Framework & Libraries
- **Bubbletea** (charmbracelet) — MVU pattern: `Init` / `Update` / `View`
- **Lipgloss** — terminal styling with the Dracula color palette (defined at top of `main.go`)
- **Bubbles** — spinner widget
- **minio-go** — S3-compatible object storage operations
- **scaleway-sdk-go** — Scaleway APIs (K8s, billing, account, registry)

### State Machine
The entire UI state lives in the `rootModel` struct. Navigation is driven by two sets of `iota` constants:

- `state` constants (`stateProfilePicker`, `stateDashboard`, `stateObjectBrowser`, `stateRegistryBrowser`, `stateBilling`) — which screen is shown
- `service` constants (`serviceObjectStorage`, `serviceK8s`, `serviceBilling`, `serviceRegistry`) — which service the dashboard cursor is on

`View()` dispatches to a `draw*()` function based on the current `state`. `handleKey()` is the central keyboard dispatcher; it delegates to `handleUp()`, `handleDown()`, `handleEnter()`, `handleEsc()` which each contain state-specific logic.

### Async I/O Pattern
All blocking work (API calls, file uploads) uses `tea.Cmd` closures that return custom message types:
- `dataMsg` — initial data load
- `bucketContentsMsg` — S3 directory listing
- `uploadProgressMsg` — streamed upload progress
- `errMsg` — any async error

Upload progress uses a global `teaProgram` variable to `Send()` messages from a background goroutine via the `progressReader` wrapper type.

### Overlays
Overlays (input prompt, upload progress, delete confirmation) are rendered conditionally in `View()` on top of the base content using boolean flags (`m.input.active`, `m.upload.active`, `m.showConfirm`).

### Client Initialization
`buildClients()` constructs both `scwClient` and `minioClient` from a named Scaleway profile loaded via `scw.LoadConfig()`. Clients are stored in `rootModel` and captured by closure in async commands.

### Styling
All colors are Dracula palette constants defined near the top of `main.go`. Do not introduce new colors without explicit user approval.

### Navigation
Vim-style throughout: `j`/`k` to move, `/` to filter, `Enter` to select, `Esc` to go back, `q` to quit.

## Key Constraints

1. Use `rootModel` for all state — no new state managers
3. Async I/O via `tea.Cmd` returning typed messages
4. Use `teaProgram.Send()` only for background goroutines (upload progress); prefer `tea.Cmd` otherwise
5. S3 ops via minio-go; Scaleway resources via scaleway-sdk-go
6. Wrap all async errors in `errMsg`

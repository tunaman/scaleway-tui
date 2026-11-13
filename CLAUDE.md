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

There are render/behavior tests in `view_instance_test.go`, `s3_download_test.go`, and `clipboard_test.go` (`go test .`).

## Architecture

The codebase is a single Go package (`package main`) split across these files:

| File | Contents |
|---|---|
| `main.go` | Constants, Dracula palette, `rootModel` struct (incl. billing + org state), `Init`, `Update`, `View`, `main` |
| `types.go` | All data structs and tea message types |
| `config.go` | `tuiConfig`, `loadTUIConfig`, `saveTUIConfig`, `buildClients` |
| `keys.go` | `handleKey`, `handleEsc`, `handleUp`, `handleDown`, `handleEnter`, `activateProfile`, `copyToClipboard` (OSC 52 copy via `tea.SetClipboard` + top-bar note) |
| `filter.go` | `filteredBuckets/RegistryImages/RegistryTags/RegistryNamespaces/Secrets/SecretVersions`, `maybeCalculateSize` |
| `util.go` | `panelBox`, `padRight`, `renderVScrollBar`, `formatBytes`, `formatEuroShort`, `parentPrefix`, `expandHome`, `safeRelPath`, `prevMonth`, `nextMonth`, `moneyToFloat`, `min`, `max` |
| `cmd_s3.go` | `fetchData`, `calculateSize`, `fetchBucketContents`, `createBucket/Folder`, `uploadFile`, `downloadFiles` (multi-file, `.part` temp + rename), `deleteEntries`, `progressReader` |
| `cmd_instance.go` | `listServersAllZones` (all zones × all states of the region), `serverFromSDK` (collects `ipamIPIDs` from `PrivateNics`), `fetchServers`, `powerServer`, `pollServersAfterDelay`, `regionZones`, `fetchServerPrivateIP` + `selectedServerPrivateIPCmd` (lazy IPAM private-IP resolution) |
| `cmd_k8s.go` | `fetchNodePools` (annotates each pool with node-type stock via `fetchServerTypesAvailability`), `fetchNodes`, `rebootNode`, `replaceNode` |
| `cmd_registry.go` | `fetchRegistryImages/Tags`, `deleteRegistryTags` |
| `cmd_billing.go` | `fetchBillingOverview`, `fetchConsumptionDetail` (aggregates by category+product), `exportBillingCSV` (date-range, project-filtered) |
| `cmd_secrets.go` | `fetchSecretVersions`, `accessSecretVersion`, `createSecretVersion`, `updateSecretVersionDesc` |
| `cmd_iam.go` | `fetchIAM` — loads all six IAM lists (users, applications, groups, policies, API keys, logs) org-scoped in one command (paginated; logs capped at `iamLogsCap`) |
| `view_k8s.go` | `drawK8sBrowser`, `renderK8sPoolPane` (incl. STOCK column), `renderK8sNodePane`, `renderK8sRebootConfirm/ReplaceConfirm` (warn when the pool's node type is scarce or out of stock) |
| `view_picker.go` | `drawProfilePicker` |
| `view_dashboard.go` | `drawDashboard`, `renderTopBar/StatusBar/Nav/Content`, `renderServers/ServerDetail` (Instances, incl. power-action confirm dialog), `renderBuckets/Clusters/Registry/Secrets/BillingPreview` |
| `view_input.go` | `renderInputOverlay` — shared input dialog for all input modes |
| `view_s3.go` | `drawObjectBrowser`, browser render helpers, `renderConfirmDialog/UploadProgress/DownloadProgress` |
| `view_registry.go` | `drawRegistryBrowser`, `renderRegistryVersionPane`, `registryPullCommand` (shared by pull overlay + copy action), tag action/delete overlays |
| `view_secrets.go` | `drawSecretsBrowser`, `renderSecretVersionDetailPane`, `renderSecretContentOverlay` |
| `view_billing.go` | `drawBilling`, `renderBillingChart/Detail/TopBar/StatusBar`, `renderBillingProjectOverlay`, `renderBillingExportOverlay` |
| `view_iam.go` | `drawIAMBrowser`, `renderIAMTabStrip`, `iamTabContent` (per-tab columns), `renderIAMPreview` (dashboard pane), `iamResolveName` |

### Framework & Libraries
- **Bubbletea** (charmbracelet) — MVU pattern: `Init` / `Update` / `View`
- **Lipgloss** — terminal styling with the Dracula color palette (defined at top of `main.go`)
- **Bubbles** — spinner widget
- **minio-go** — S3-compatible object storage operations
- **scaleway-sdk-go** — Scaleway APIs (K8s, instance stock, billing, account, registry, secrets, IAM)

### State Machine
The entire UI state lives in the `rootModel` struct. Navigation is driven by two sets of `iota` constants:

- `state` constants (`stateProfilePicker`, `stateDashboard`, `stateObjectBrowser`, `stateK8sBrowser`, `stateRegistryBrowser`, `stateSecretsBrowser`, `stateBilling`, `stateIAMBrowser`) — which screen is shown
- `service` constants (`serviceInstance`, `serviceObjectStorage`, `serviceK8s`, `serviceBilling`, `serviceRegistry`, `serviceSecrets`, `serviceIAM`) — which service the dashboard cursor is on

The IAM browser (`stateIAMBrowser`) is read-only and organization-scoped. It has six sub-tabs selected by `m.iamTab` (`iamTabUsers` … `iamTabLogs`); `←/→` and `Tab/Shift+Tab` switch tabs, `/` filters the active tab. All six lists are fetched once (cached via `m.iamLoaded`) and everything after is client-side. Principals/bearers/actors are resolved to display names via the `m.iamNames` ID→name map built in the `iamDataMsg` handler.

The Instances service (`serviceInstance`) is a dashboard-level view only — no dedicated `state` constant. It renders a server list + SERVER INFO detail pane (`renderServers`) with power actions (`O`/`S`/`R`) that set `m.instConfirmAction` and show a confirm dialog; after a successful action the list refetches and polls every 3s while any server is `starting`/`stopping` (`serverPollTickMsg`).

**Private IPs (IPAM):** modern routed-IP servers carry no private address on the NIC — only `ipam_ip_ids` (collected into `instanceServer.ipamIPIDs` by `serverFromSDK`). The address is resolved lazily via the IPAM API: `selectedServerPrivateIPCmd()` fires `fetchServerPrivateIP` for the currently selected server (first IPv4 of its NIC IPs) when it isn't already cached in `m.privateIPs` or in flight (`m.privateIPLoadingID`). It is triggered on initial load (`dataMsg`/`serversMsg`) and on every cursor move in the Instances list. The detail pane shows the resolved address, a faint "Resolving…" while in flight, or "—" otherwise. Best-effort: a lookup failure just leaves "—".

`View()` dispatches to a `draw*()` function based on the current `state`. `handleKey()` is the central keyboard dispatcher; it delegates to `handleUp()`, `handleDown()`, `handleEnter()`, `handleEsc()` which each contain state-specific logic.

### Async I/O Pattern
All blocking work (API calls, file uploads) uses `tea.Cmd` closures that return custom message types:
- `dataMsg` — initial data load
- `bucketContentsMsg` — S3 directory listing
- `uploadProgressMsg` — streamed upload progress
- `downloadProgressMsg` — streamed download progress (per-file byte counts + i/N file counter; `done` carries the status-bar summary)
- `k8sNodePoolsMsg` — node pools loaded for a cluster
- `k8sNodesMsg` — nodes loaded for a pool
- `k8sNodeRebootedMsg` — node reboot confirmed
- `serversMsg` — Instances list loaded (all zones × states of the active region)
- `serverActionDoneMsg` — power action accepted; triggers `fetchServers()`
- `serverPollTickMsg` — periodic refresh while a server is starting/stopping
- `serverPrivateIPMsg` — one server's IPAM-resolved private IP ("" when none found)
- `iamDataMsg` — all six IAM lists loaded (sets `stateIAMBrowser`, builds the ID→name map)
- `clipboardNoteClearedMsg` — clears the "Copied → clipboard" top-bar note after `clipboardNoteDuration`
- `errMsg` — any async error

Upload progress uses a global `teaProgram` variable to `Send()` messages from a background goroutine via the `progressReader` wrapper type.

### Clipboard (OSC 52)
Copy actions call `m.copyToClipboard(text)` (keys.go), which batches `tea.SetClipboard(text)` (Bubbletea v2's built-in OSC 52 command — the framework writes the escape sequence to the TTY itself) with a 2s `tea.Tick` that clears `m.clipboardNote`. The note ("✓ Copied → clipboard") is rendered in the top bar's right section (renderTopBar). Copy hotkeys: `C` in Instances (server IP), K8s nodes pane (node IP), IAM API-keys tab (access key), registry pull dialog, and secret content viewer; `Y` in the object browser (object key). Terminals without OSC 52 support simply ignore the sequence.

### Overlays
Overlays are rendered conditionally in `View()` on top of the base content using boolean flags:
- `m.input.active` — text input dialog (`view_input.go`); covers all `inputMode` variants
- `m.upload.active` — upload progress bar (`view_s3.go`)
- `m.download.active` — download progress bar (`view_s3.go`); `X` in the object browser asks for a destination dir via `inputModeDownload`
- `m.showConfirm` — S3 delete confirmation (`view_s3.go`)
- `m.regTagActionOverlay` — registry pull-command dialog (`view_registry.go`)
- `m.regConfirmDeleteTags` — registry tag delete confirmation (`view_registry.go`)
- `m.secShowContent` — secrets version content viewer (`view_secrets.go`)
- `m.k8sConfirmReboot` — node reboot confirmation dialog (`view_k8s.go`); `R` key
- `m.instConfirmAction != instActionNone` — server power-action confirmation dialog (`view_dashboard.go`); `O`/`S`/`R` keys
- `m.billingProjectOverlay` — billing project picker (`view_billing.go`); `p/P` key
- `m.billingExportOverlay` — billing CSV export date-range picker (`view_billing.go`); `e/E` key

### Client Initialization
`buildClients()` constructs both `scwClient` and `minioClient` from a named Scaleway profile loaded via `scw.LoadConfig()`. Clients are stored in `rootModel` and captured by closure in async commands.

### Styling
All colors are Dracula palette constants defined near the top of `main.go`. Do not introduce new colors without explicit user approval.

**Selected/highlighted list rows must be rendered as plain (uncolored) text.** When a row is wrapped in a `Background(colBg3)` style, any inline ANSI color reset baked into the row string (per-cell `Foreground(...).Render(...)`, `highlightMatch`, etc.) turns the background off partway across, so the highlight stops short of the right edge. Build cursor rows from plain strings so the background spans the full width — either assemble a separate uncolored `plainRowStr` (see `renderClusters`/`renderRegistry` in `view_dashboard.go`) or strip codes from an already-colored row with `stripANSI` (`util.go`, used by `view_iam.go`). Non-selected rows keep their per-column colors.

### Navigation
Vim-style throughout: `j`/`k` to move, `/` to filter, `Enter` to select, `Esc` to go back, `q` to quit.

## Key Constraints

1. Use `rootModel` for all state — no new state managers
3. Async I/O via `tea.Cmd` returning typed messages
4. Use `teaProgram.Send()` only for background goroutines (upload progress); prefer `tea.Cmd` otherwise
5. S3 ops via minio-go; Scaleway resources via scaleway-sdk-go
6. Wrap all async errors in `errMsg`

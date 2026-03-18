# scw-tui — Scaleway Terminal UI

A keyboard-driven terminal UI for managing your [Scaleway](https://www.scaleway.com) cloud resources, built with [Bubbletea](https://github.com/charmbracelet/bubbletea) and the Dracula colour theme.

```bash
  ██████  ██████ ██     ██      ████████ ██    ██ ██
 ██      ██      ██     ██         ██    ██    ██ ██
  █████  ██      ██  █  ██  ███    ██    ██    ██ ██
      ██ ██      ██ ███ ██         ██    ██    ██ ██
 ██████   ██████  ███ ███          ██     ██████  ██
```

---

## Features

- **Multi-profile support** — pick any profile from your existing `~/.config/scw/config.yaml`
- **Object Storage browser** — navigate buckets and folders, view object sizes, upload, create folders/buckets, and delete objects
- **Kubernetes overview** — list Kapsule clusters with status and version
- **Container Registry browser** — browse namespaces, images, and tags; copy pull commands
- **Secrets Manager** — list secrets, browse versions, view secret content, add new versions, and update version descriptions
- **Billing view** — inspect current-period costs, filter by project, export date-range to CSV
- **Project switcher** — switch between Scaleway projects without leaving the TUI
- **Vim-style keyboard navigation** — `j/k`, `/` to filter, and single-key actions throughout

## Prerequisites

- Go 1.21+
- A Scaleway account with at least one profile configured in `~/.config/scw/config.yaml`
  (set up with `scw init` or by editing the file manually)

## Installation

```bash
git clone https://github.com/<your-username>/scw-tui.git
cd scw-tui
make build
```

The binary is placed at `bin/scw-tui`.

### Docker

```bash
docker build -t scw-tui .
docker run -it --rm \
  -v "$HOME/.config/scw:/root/.config/scw:ro" \
  scw-tui
```

## Usage

```bash
./bin/scw-tui
```

On first launch you will be presented with a profile picker. Select a profile and press `Enter` to connect.

### Keyboard shortcuts

#### Global

| Key | Action |
| ----- | -------- |
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `Enter` | Select / open |
| `/` | Filter list |
| `F5` | Refresh |
| `Esc` | Back / cancel / clear filter |
| `q` | Quit |

#### Object Storage

| Key | Action |
| ----- | -------- |
| `Tab` | Switch focus pane |
| `c` | Create bucket (dashboard) / folder (browser) |
| `u` | Upload file |
| `d` | Delete selected object(s) |
| `a` | Select / deselect all |
| `Space` | Toggle selection |
| `←` / `→` | Scroll name column |

#### Container Registry

| Key | Action |
| ----- | -------- |
| `Tab` | Switch between images and tags panes |
| `Enter` | Show pull command for selected tag |
| `Space` | Toggle tag selection |
| `A` | Select / deselect all tags |
| `D` | Delete selected tag(s) |

#### Billing

| Key | Action |
| ----- | -------- |
| `←` / `→` | Previous / next month |
| `j` / `k` | Navigate detail rows |
| `P` | Open project picker (filter by project or all) |
| `E` | Open date-range picker and export to CSV |
| `F5` | Refresh |

#### Secrets Manager

| Key | Action |
| ----- | -------- |
| `Enter` | View secret content for selected version |
| `n` | Add a new secret version |
| `u` | Update description of selected version |

## Configuration

scw-tui stores only UI preferences (last active profile) in `~/.config/scw-tui/config.json`. Credentials are never duplicated — they are read exclusively from the official Scaleway config.

## Development

```bash
# Build
make build

# Clean
make clean

# Tidy dependencies
go mod tidy
```

The codebase is a single `package main` split across focused files (`main.go`, `keys.go`, `filter.go`, `util.go`, `types.go`, `config.go`, `cmd_*.go`, `view_*.go`). See [CLAUDE.md](CLAUDE.md) for an architectural overview.

## Tech stack

| Library | Purpose |
| --------- | --------- |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | UI components (spinner, etc.) |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling (Dracula theme) |
| [scaleway/scaleway-sdk-go](https://github.com/scaleway/scaleway-sdk-go) | Scaleway API (K8s, billing, account, registry, secrets) |
| [minio/minio-go](https://github.com/minio/minio-go) | S3-compatible object storage |

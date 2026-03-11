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
- **Object Storage browser** — navigate buckets and folders, view object sizes, upload, download, create folders/buckets, and delete objects
- **Kubernetes overview** — list Kapsule clusters with status and version
- **Billing view** — inspect current-period costs per project
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

| Key | Action |
| ----- | -------- |
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `Enter` | Select / open |
| `Tab` | Switch focus pane |
| `/` | Filter list |
| `c` | Create bucket / folder |
| `u` | Upload file |
| `d` | Delete selected object |
| `a` | Select all |
| `Esc` | Back / cancel |
| `q` | Quit |

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

All application logic lives in `main.go`. See [.github/copilot-instructions.md](.github/copilot-instructions.md) for an architectural overview.

## Tech stack

| Library | Purpose |
| --------- | --------- |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI framework |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | UI components (spinner, etc.) |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling (Dracula theme) |
| [scaleway/scaleway-sdk-go](https://github.com/scaleway/scaleway-sdk-go) | Scaleway API (K8s, billing, account) |
| [minio/minio-go](https://github.com/minio/minio-go) | S3-compatible object storage |

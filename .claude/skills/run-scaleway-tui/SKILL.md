---
name: run-scaleway-tui
description: Build, run, and drive scw-tui (the Scaleway terminal UI). Use when asked to start scw-tui, build it, take a screenshot of a screen, drive/interact with the running TUI, or verify a UI change (Object Storage, K8s, Billing, Registry, Secrets, IAM).
---

scw-tui is a full-screen Bubbletea (alt-screen) terminal UI. It can't be driven
by piping stdin — it takes over the terminal — so drive it through
`.claude/skills/run-scaleway-tui/driver.sh`, which wraps it in a **tmux** session
and exposes `build` / `start` / `connect` / `keys` / `cap` / `shot` / `stop`.
"Screenshots" are plain-text `capture-pane` dumps written to a file.

All paths below are relative to the repo root (the unit).

Verified on **macOS (darwin) with Homebrew Go** — this is the dev environment
this repo lives in, not a Linux CI container.

## Prerequisites

- **Go** (Homebrew). Note: this machine's shell `GOROOT` is stale and `go`
  fails by default (see Gotchas) — `driver.sh build` resolves the real GOROOT for you.
- **tmux** — required to drive the TUI:

```bash
brew install tmux
```

- **A Scaleway config** at `~/.config/scw/config.yaml` with at least one profile
  **and valid credentials**. Without it the app exits at launch ("no profiles
  found"); with an unauthenticated profile you reach the picker but `connect`
  fails on the first API call. Set up via `scw init`.

## Build

```bash
.claude/skills/run-scaleway-tui/driver.sh build
```

Produces `bin/scw-tui`. (It sets `GOROOT` to the newest `/opt/homebrew/Cellar/go/*/libexec`.)

## Run (agent path)

Drive everything through the driver. A full run to the IAM screen:

```bash
D=.claude/skills/run-scaleway-tui/driver.sh
"$D" start          # launch in tmux, wait for the profile picker
"$D" connect        # Enter on the picker → connect the active profile, wait for dashboard
"$D" open-iam       # open the IAM browser (Users tab)
"$D" shot /tmp/iam.txt   # save the screen to a file
sed -n '5,14p' /tmp/iam.txt   # look at it
"$D" stop
```

Drive any screen with the primitives:

| command | what it does |
|---|---|
| `build` | Build `bin/scw-tui` with a resolved Homebrew GOROOT |
| `start` | Launch in a detached tmux session (`160x45`), wait for the picker |
| `connect` | Press Enter on the picker, wait for the dashboard (`SERVICES`) |
| `open-iam` | From a **freshly connected** dashboard, open the IAM Users tab |
| `keys <k…>` | Forward to `tmux send-keys` (e.g. `keys Down Down Enter`) |
| `keys -l <text>` | Send **literal** text — needed for filter input (e.g. `keys -l terra`) |
| `cap` | Print the current screen |
| `cape` | Print the screen **with** escape codes (inspect colors / selected-row bg) |
| `shot <file>` | Write the current screen (plain text) to `<file>` |
| `wait <regex> [s]` | Poll until the screen matches `<regex>` (default 30s) |
| `stop` | Quit and kill the tmux session |

Keys inside the app (vim-style): `j`/`k` or `Down`/`Up` move, `Enter` selects,
`/` filters, `Esc` goes back / clears filter, `q` quits, `F5` refreshes, `Tab`
toggles nav↔content focus (and switches panes inside browsers).

IAM browser specifics: `Left`/`Right` (or `Tab`/`shift+tab`) switch the six tabs
(Users, Applications, Groups, Policies, API keys, Logs); `/` filters the active tab.

## Run (human path)

```bash
./bin/scw-tui   # opens the profile picker; pick a profile, Enter. q to quit.
```

Useless to an agent (needs an interactive terminal) — use the driver instead.

## Test

There are no automated tests in this repo. Verify changes by driving the app
(above) and reading the captured screen. For static checks:

```bash
GOROOT="$(ls -d /opt/homebrew/Cellar/go/*/libexec | sort -V | tail -1)" go vet ./...
golangci-lint run
```

## Gotchas

- **`go` fails out of the box: `cannot find GOROOT directory: …/go/1.26.3/libexec`.**
  The shell's `GOROOT` points at an old Homebrew version that no longer exists;
  Homebrew Go bumps the version over time. `driver.sh` resolves the current one
  (`ls -d /opt/homebrew/Cellar/go/*/libexec | sort -V | tail -1`). For raw `go`
  commands, prefix `GOROOT=$(ls -d /opt/homebrew/Cellar/go/*/libexec | sort -V | tail -1)`.
- **tmux/go aren't on a non-login PATH.** They're in `/opt/homebrew/bin`; the
  driver prepends it. Raw commands may need `export PATH="/opt/homebrew/bin:$PATH"`.
- **After `connect`, focus starts on the NAV pane** (Object Storage selected).
  So to reach a service, send `Down`s **first**, then `Tab` to focus content,
  then `Enter`. A leading `Tab` sends focus to content and your `Down`s move the
  list cursor instead of the service selection — that's why `open-iam` must be
  called right after `connect`.
- **Filter text must be sent literally: `keys -l <text>`.** Plain `keys terra`
  can be mis-parsed by tmux as key names; `-l` sends the raw string into the
  active `/` filter.
- **Real credentials required.** The picker renders offline, but `connect` and
  every screen hit the live Scaleway API — a broken/unauthorized profile fails
  there, not at launch.
- **It's an alt-screen app.** `capture-pane -p` is the screen; there is no
  scrollback of app output. Use `-e`/`cape` to inspect ANSI (e.g. to check a
  selected row's highlight background spans the full width).

## Troubleshooting

- **`missing bin/scw-tui — run: … build`** on `start`: build first with `driver.sh build`.
- **`wait: timed out … waiting for /SERVICES/`** after `connect`: the profile
  failed to authenticate, or there's no network. Check `~/.config/scw/config.yaml`
  credentials; run `./bin/scw-tui` by hand to see the error pane.
- **`start` shows `no profiles found` and exits:** no `~/.config/scw/config.yaml`
  — run `scw init`.
- **Navigation lands on the wrong service:** you pressed `Tab` before the `Down`s
  (see Gotchas). `stop`, then `start`/`connect`/`open-iam` fresh.

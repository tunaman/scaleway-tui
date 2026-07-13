#!/usr/bin/env bash
# Driver for scw-tui — build and drive the Bubbletea TUI via tmux so an agent
# can send keystrokes and capture the screen. See ../run-scaleway-tui/SKILL.md.
#
# Subcommands:
#   build              Build ./bin/scw-tui with a resolved Homebrew GOROOT.
#   start              Launch the binary in a detached tmux session; wait for the
#                      profile picker (or the "no profiles" error) to appear.
#   connect            Press Enter on the picker and wait for the dashboard.
#   open-iam           From a freshly-connected dashboard, open the IAM browser
#                      (Users tab). MUST be called right after `connect` — it
#                      relies on focus starting on the nav pane.
#   keys <k...>        Forward args to `tmux send-keys` (e.g. keys Down Down Enter).
#   cap                Print the current screen (capture-pane -p).
#   cape               Print the screen WITH escape codes (capture-pane -pe) —
#                      use to inspect colors / selected-row background.
#   shot <file>        Write the current screen (plain text) to <file>.
#   wait <regex> [s]   Poll until the screen matches <regex> (default 30s).
#   stop               Kill the tmux session.
set -euo pipefail

# Homebrew tools (tmux, go, brew) live here and are often not on a non-login PATH.
export PATH="/opt/homebrew/bin:$PATH"

SESSION="${SCWTUI_SESSION:-scwtui}"
BIN="${SCWTUI_BIN:-bin/scw-tui}"
COLS="${SCWTUI_COLS:-160}"
ROWS="${SCWTUI_ROWS:-45}"

# Newest Homebrew Go install; the shell's own $GOROOT is frequently stale.
_goroot() { ls -d /opt/homebrew/Cellar/go/*/libexec 2>/dev/null | sort -V | tail -1; }

cmd_build() {
  local gr; gr="$(_goroot)"
  if [ -n "$gr" ]; then export GOROOT="$gr"; export PATH="$gr/bin:$PATH"; fi
  go build -o "$BIN" .
  echo "built $BIN (GOROOT=${gr:-<default>})"
}

cmd_wait() {
  local re="${1:?usage: wait <regex> [seconds]}"; local secs="${2:-30}"
  if ! timeout "$secs" bash -c "until tmux capture-pane -t '$SESSION' -p | grep -qiE '$re'; do sleep 0.3; done"; then
    echo "wait: timed out after ${secs}s waiting for /$re/" >&2
    tmux capture-pane -t "$SESSION" -p 2>/dev/null | tail -20 >&2 || true
    return 1
  fi
}

cmd_start() {
  [ -x "$BIN" ] || { echo "missing $BIN — run: $0 build" >&2; return 1; }
  tmux kill-session -t "$SESSION" 2>/dev/null || true
  tmux new-session -d -s "$SESSION" -x "$COLS" -y "$ROWS" "./$BIN"
  cmd_wait 'SELECT PROFILE|SERVICES|no profiles found' 12
  cmd_cap
}

cmd_connect() {
  tmux send-keys -t "$SESSION" Enter
  cmd_wait 'SERVICES' 40
}

# Open the IAM browser from a fresh dashboard. After connect the focus is on the
# nav pane and Object Storage (index 0) is selected, so: move down 5 to IAM,
# Tab to focus the content pane, Enter to load. Do NOT press Tab before this.
cmd_open_iam() {
  tmux send-keys -t "$SESSION" Down Down Down Down Down; sleep 0.4
  tmux send-keys -t "$SESSION" Tab;   sleep 0.3
  tmux send-keys -t "$SESSION" Enter; sleep 0.3
  cmd_wait 'USER +TYPE' 60
}

cmd_keys() { tmux send-keys -t "$SESSION" "$@"; }
cmd_cap()  { tmux capture-pane -t "$SESSION" -p; }
cmd_cape() { tmux capture-pane -t "$SESSION" -pe; }
cmd_shot() { tmux capture-pane -t "$SESSION" -p > "${1:?usage: shot <file>}"; echo "wrote $1"; }
cmd_stop() { tmux send-keys -t "$SESSION" q 2>/dev/null || true; sleep 0.2; tmux kill-session -t "$SESSION" 2>/dev/null || true; echo "stopped"; }

case "${1:-}" in
  build)   shift; cmd_build "$@";;
  start)   shift; cmd_start "$@";;
  connect) shift; cmd_connect "$@";;
  open-iam) shift; cmd_open_iam "$@";;
  keys)    shift; cmd_keys "$@";;
  cap)     shift; cmd_cap "$@";;
  cape)    shift; cmd_cape "$@";;
  shot)    shift; cmd_shot "$@";;
  wait)    shift; cmd_wait "$@";;
  stop)    shift; cmd_stop "$@";;
  *) echo "usage: driver.sh {build|start|connect|open-iam|keys <k..>|cap|cape|shot <f>|wait <re> [s]|stop}"; exit 2;;
esac

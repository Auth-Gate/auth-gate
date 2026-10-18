#!/usr/bin/env bash
# Hermetic end-to-end test for tools/wt. Everything happens inside a temp dir.
set -euo pipefail

wt="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo="$(cd "$wt/../.." && pwd)"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

export CLAUDE_PROJECTS_DIR="$tmp/claude-projects"
export WT_GATE=true
export GIT_AUTHOR_NAME=e2e GIT_AUTHOR_EMAIL=e2e@example.com
export GIT_COMMITTER_NAME=e2e GIT_COMMITTER_EMAIL=e2e@example.com
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export HOME="$tmp/home"
mkdir -p "$HOME"

fail=0
ok() { echo "ok - $1"; }
nok() { echo "not ok - $1"; fail=1; }
assert() {
  local msg="$1"
  shift
  if "$@" >/dev/null 2>&1; then ok "$msg"; else nok "$msg"; fi
}

# --- Seeding: a throwaway remote whose main carries the tools under test ---
seed_remote() {
  local seed="$tmp/seed"
  git init -q -b main "$seed"
  mkdir -p "$seed/tools" "$seed/docs/infra"
  cp -R "$repo/tools/wt" "$seed/tools/wt"
  rm -rf "$seed/tools/wt/test"
  [ -d "$repo/docs/infra/templates" ] && cp -R "$repo/docs/infra/templates" "$seed/docs/infra/templates"
  [ -d "$repo/.claude" ] && cp -R "$repo/.claude" "$seed/.claude"
  echo '{"name":"e2e","private":true}' >"$seed/package.json"
  git -C "$seed" add -A
  git -C "$seed" commit -q -m "chore: seed"
  git init -q --bare -b main "$tmp/remote.git"
  git -C "$seed" push -q "$tmp/remote.git" main
}
seed_remote

# --- lib.sh ---
# shellcheck source=../lib.sh
. "$wt/lib.sh"
assert "dir_of maps slash to dash" test "$(dir_of feat/x)" = "feat-x"
assert "key_of replaces non-alphanumerics" test "$(key_of /a/b.c)" = "-a-b-c"

# --- init.sh ---
"$wt/init.sh" "$tmp/remote.git" "$tmp/ws" >/dev/null
assert "init: bare repo" test -f "$tmp/ws/.bare/HEAD"
assert "init: wt_root from a subfolder" test "$(cd "$tmp/ws/main" && wt_root)" = "$tmp/ws"
assert "init: .git pointer" grep -qx 'gitdir: ./.bare' "$tmp/ws/.git"
assert "init: main worktree" test "$(git -C "$tmp/ws/main" branch --show-current)" = main
assert "init: orchestrator orphan" test "$(git -C "$tmp/ws/orchestrator" rev-list --count HEAD)" = 1
assert "init: orchestrator board" test -f "$tmp/ws/orchestrator/BOARD.md"
assert "init: orchestrator gitignore" grep -qx '/local' "$tmp/ws/orchestrator/.gitignore"
assert "init: local symlinks" test -L "$tmp/ws/main/local" -a -L "$tmp/ws/orchestrator/local"
assert "init: .bare/claude link" test "$(readlink "$tmp/ws/.bare/claude")" = "$CLAUDE_PROJECTS_DIR/$(cd "$tmp/ws" && bare_key)"
assert "init: idempotent refusal" ! "$wt/init.sh" "$tmp/remote.git" "$tmp/ws"

# --- new.sh --- (appended by a later task)
# --- merge.sh --- (appended by a later task)
# --- rm.sh --- (appended by a later task)
# --- status.sh --- (appended by a later task)
# --- claude-link.sh --- (appended by a later task)

[ "$fail" = 0 ]

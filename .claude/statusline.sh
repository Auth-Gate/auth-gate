#!/usr/bin/env bash
# Statusline: "<branch> | ctx <N>% | $<cost>"; also records N for the context guard.
set -euo pipefail

ctx_dir="${CLAUDE_CTX_DIR:-/tmp/claude-ctx}"
mkdir -p "$ctx_dir"

input="$(cat)"
session="$(jq -r '.session_id // "unknown"' <<<"$input")"
dir="$(jq -r '.workspace.current_dir // "."' <<<"$input")"
pct="$(jq -r '(.context_window.used_percentage // 0) | floor' <<<"$input")"
cost="$(jq -r '.cost.total_cost_usd // 0' <<<"$input")"

branch="$(git -C "$dir" branch --show-current 2>/dev/null || true)"
[ -n "$branch" ] || branch="-"

session="${session//[^A-Za-z0-9_-]/_}"
echo "$pct" >"$ctx_dir/$session"

LC_ALL=C printf '%s | ctx %s%% | $%.2f\n' "$branch" "$pct" "$cost"

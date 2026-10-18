#!/usr/bin/env bash
# Rebase a feature branch on main, run the gate, fast-forward main:
#   merge.sh <branch> [--no-push]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$here/lib.sh"

usage="usage: merge.sh <branch> [--no-push]"
branch=""
push=1
for arg in "$@"; do
  case "$arg" in
    --no-push) push=0 ;;
    -*) die "unknown option: $arg ($usage)" ;;
    *)
      [ -z "$branch" ] || die "$usage"
      branch="$arg"
      ;;
  esac
done
[ -n "$branch" ] || die "$usage"

root="$(wt_root)"
dir="$(dir_of "$branch")"
main="$root/$BASE_BRANCH"
gate="${WT_GATE:-pnpm run check}"
[ -d "$root/$dir" ] || die "no worktree at $root/$dir"
[ -d "$main" ] || die "no worktree at $main"

step "Checking worktrees are clean"
[ -z "$(git -C "$root/$dir" status --porcelain)" ] || die "$dir has uncommitted changes"
[ -z "$(git -C "$main" status --porcelain)" ] || die "$BASE_BRANCH has uncommitted changes"

if git -C "$root" remote get-url origin >/dev/null 2>&1; then
  step "Fetching origin"
  git -C "$root" fetch -q origin
fi
if git -C "$main" rev-parse --abbrev-ref '@{u}' >/dev/null 2>&1; then
  step "Updating $BASE_BRANCH"
  git -C "$main" pull -q --ff-only
fi

step "Rebasing $branch onto $BASE_BRANCH"
git -C "$root/$dir" rebase "$BASE_BRANCH"

step "Running gate: $gate"
(cd "$root/$dir" && eval "$gate")

step "Fast-forwarding $BASE_BRANCH to $branch"
git -C "$main" merge -q --ff-only "$branch"

if [ "$push" = 1 ]; then
  step "Pushing $BASE_BRANCH"
  git -C "$main" push -q origin "$BASE_BRANCH"
else
  warn "--no-push: $BASE_BRANCH not pushed"
fi

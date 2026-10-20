# feat/rm-after-rebase

## Goal
`rm.sh <branch>` (without `--force`) must clean up a branch that `merge.sh` rebased and merged.
Today it fails: `git branch -d` refuses because the local branch differs from the stale
`origin/<branch>` (pushed before the rebase), and the remote-branch check at
`tools/wt/rm.sh:66` (`merge-base --is-ancestor origin/$branch main`) keeps the old remote branch.
Seen on feat/auth-gate, 2026-10-20: needed `rm.sh --force`.

## Constraints
- Do not weaken the safety: a branch with commits truly not in `main` must still be refused
  without `--force`, both locally and on origin.
- Pick one approach and justify it in the handoff, e.g.:
  a) `merge.sh` pushes the rebased branch (`--force-with-lease`) before merging, so origin
     matches what was merged; or
  b) `rm.sh` decides "merged" by content (e.g. `git cherry main origin/<branch>` showing no `+`
     lines) and then uses `branch -D` / remote delete.
- Bash, shellcheck clean, matches the style of `tools/wt/*.sh`.
- Update `docs/infra/02-worktree-lifecycle.md` if behaviour changes.

## Definition of done
- e2e test in `tools/wt/test/e2e.sh`: a branch pushed, then rebased and merged by `merge.sh`,
  is fully removed by `rm.sh` without `--force` (worktree, local branch, remote branch).
- Existing refusal tests ("rm: refuses unmerged without --force", "rm: unmerged remote branch
  kept") still pass.
- `pnpm run check` passes.

## References
- tools/wt/rm.sh, tools/wt/merge.sh, tools/wt/test/e2e.sh
- docs/infra/02-worktree-lifecycle.md

# 2. Coordination state lives on an orphan `orchestrator` branch

Date: 2026-10-18

Status: accepted

## Context

Someone must decide which worktree builds what, when a branch is merged, and when a
worktree is removed. That state (a board of assignments, briefs, handoffs) changes many
times a day. It needs history, it must be backed up, and it should be shown as part of
the reproducible design. It has nothing to do with product history.

Options considered:

- A folder in `main`: every board change becomes a commit in the public product history,
  and feature branches would carry stale copies of the board.
- `local/`: no history, not backed up, and private by definition.
- A separate repository: a second remote and a second set of tooling for a few files.

## Decision

Keep coordination state on `orchestrator`, an orphan branch with no shared history with
`main`. It is checked out in the `orchestrator/` worktree, committed after every change,
and pushed to the remote. It is never merged into `main`.

It holds `CLAUDE.md` (the orchestrator role), `BOARD.md`, `handoffs/<dir>/` and a copy of
`.claude/`. Feature sessions read and write it through `../orchestrator/`.

A dedicated Claude session runs in that worktree. It coordinates and prints commands for
the human to paste. It does not write product code.

## Consequences

- `main` stays a clean, publishable history.
- The orchestrator's history is versioned and backed up like any branch.
- `.claude/` is duplicated on two branches. `tools/wt/status.sh` warns when
  `orchestrator/.claude` differs from `main/.claude`. A symlink was rejected because Claude
  Code treats settings that point outside the project as external.
- Creating an orphan worktree needs a workaround on git older than 2.42 (no
  `git worktree add --orphan`): add a detached worktree, then `git checkout --orphan`.

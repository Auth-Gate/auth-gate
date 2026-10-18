# 1. Bare repository with one worktree per branch

Date: 2026-10-18

Status: accepted

## Context

Several features are built at the same time, each in its own editor window and its own
Claude Code session. A normal checkout has one working tree, so parallel work means
stashing, switching branches, or cloning the repository several times. Clones duplicate
the object database and drift apart. Switching branches under a running Claude session
confuses it.

Git worktrees give many working trees over one object database. Where the repository
lives decides how tidy the layout is: with a normal checkout, the first worktree is the
repository and every other worktree is nested or placed beside it unevenly.

## Decision

The workspace root is not a checkout. It contains:

- `.bare/`: the git repository (`core.bare = true`).
- `.git`: one line, `gitdir: ./.bare`, so git commands work from the root.
- `main/`, `orchestrator/`, `feat-<name>/`: one worktree per branch, all siblings.
- `local/`: private, gitignored, symlinked into every worktree.

The directory name is the branch name with `/` replaced by `-`, with no exceptions.
Every feature forks from `origin/main` and returns by rebase plus fast-forward.
Scripts in `tools/wt/` create, merge and remove worktrees.

## Consequences

- Every branch has a predictable path. Scripts and humans map branch to folder without a
  lookup, and sibling worktrees reach each other with `../<dir>`.
- One object database, so a new worktree costs only its files.
- `main` stays linear, which keeps the public history readable and `git bisect` usable.
- Deleting `.bare/` destroys every worktree. It must never be touched by hand.
- Claude must not run at the workspace root; sessions run inside a worktree folder.
- Tools that assume `.git` is a directory inside the project may need care. Inside a
  worktree, `.git` is a file pointing into `.bare/worktrees/<dir>`.

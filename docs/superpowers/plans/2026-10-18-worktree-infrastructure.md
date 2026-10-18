# Worktree Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `/home/work/auth-gate` into a bare-repo + worktree workspace with an orchestrator branch, lifecycle scripts, Claude session continuity hooks, an empty pnpm workspace with a real gate, CI, and documentation that lets a fresh Claude session rebuild the same infrastructure for another project.

**Architecture:** The workspace root holds `.bare` (the repository), `main/`, `orchestrator/` (orphan branch, state only) and `feat-*/` worktrees. Scripts in `main/tools/wt/` implement the lifecycle; they are tested end-to-end against a throwaway remote in a temp directory. Hooks in `main/.claude/` inject the assignment and last handoff into every new session and warn when context is nearly full.

**Tech Stack:** bash (shellcheck-clean), git 2.39.5, pnpm 10.13.1, Node 22, jq 1.6, Claude Code 2.1.301, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-18-worktree-infrastructure-design.md`

## Global Constraints

- Every script: `#!/usr/bin/env bash`, `set -euo pipefail`, passes `shellcheck`.
- Directory name = branch name with `/` replaced by `-` (`feat/x` ↔ `feat-x`). Baseline branch is `main`.
- Code, comments, docs, commit messages in English. Commits follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `test:`).
- Scripts never force-push and never delete a non-reproducible thing without `--force`.
- Steps marked **HUMAN** are run by the human, never by Claude: anything that pushes to GitHub, changes the machine's toolchain, or needs an interactive Claude session.
- git is 2.39.5: `git worktree add --orphan` does not exist; orphans are made with `git checkout --orphan` inside a detached worktree.
- `pnpm` must resolve to the npm-installed 10.13.1 binary, not the corepack shim (Task 0).
- Worktree scripts read `CLAUDE_PROJECTS_DIR` (default `$HOME/.claude/projects`) and `WT_GATE` (default `pnpm run check`) so tests can redirect them. Hooks read `CLAUDE_CTX_DIR` (default `/tmp/claude-ctx`).
- Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. `rm.sh` on a branch whose worktree folder was already deleted by hand: expected to prune the stale worktree entry and still delete the branch, not crash on `git worktree remove`. Test added to Task 8.
2. `new.sh` for a branch that is already checked out in another worktree: expected to fail with git's own message and leave nothing half-created (no `local` symlink, no Claude symlink). Test added to Task 6.
3. `merge.sh` when the feature worktree has uncommitted changes: expected to refuse before rebasing, so a dirty tree is never rebased. Test added to Task 7.
4. `claude-link.sh` when the worktree's project directory already exists as a real, non-empty directory (a session was started before linking): expected to keep it as `<key>.bak-<timestamp>` and link, never delete. Test added to Task 9.
5. `session-start.sh` in a worktree that has no BOARD row and no handoffs folder: expected to print the "no assignment" line and exit 0, never a stack of errors that poison the first context. Test added to Task 10.

---

### Task 0: Fix pnpm on this machine (HUMAN)

**Files:** none.

- [ ] **Step 1 (HUMAN): remove only the pnpm corepack shims, keep yarn's**

```bash
corepack disable pnpm
npm install -g pnpm@10.13.1
```

- [ ] **Step 2: Verify**

Run: `which pnpm && pnpm --version && pnpm store path`
Expected: a path ending in `/bin/pnpm`, `10.13.1`, and `/home/ongdev2k2/.local/share/pnpm/store/v10`. No corepack stack trace.

---

### Task 1: First commit and conversion to the bare layout

**Files:**
- Create: `.gitignore`, `LICENSE`, `README.md` (two paragraphs: what auth-gate is, "infrastructure under construction, see docs/infra"), keep `docs/superpowers/specs/*.md` and `docs/superpowers/plans/*.md`.

**Interfaces:**
- Produces: workspace root `/home/work/auth-gate` with `.bare/`, `.git` (file), `main/` on branch `main`, `local/` shared.

- [ ] **Step 1: Write `.gitignore` with exactly the spec section 10 contents**

```gitignore
local/
node_modules/
.env
.env.*
!.env.example
.claude/settings.local.json
bruno/environments/*.yml
!bruno/environments/*.example.yml
```

- [ ] **Step 2: Write `LICENSE` (MIT, `Copyright (c) 2026 Pham Dang Huy`) and `README.md`**

- [ ] **Step 3: Verify `local/` is ignored**

Run: `cd /home/work/auth-gate && git status --short`
Expected: lists `.gitignore`, `LICENSE`, `README.md`, `docs/`; does not list `local/`.

- [ ] **Step 4: First commit on `main`**

```bash
git add .gitignore LICENSE README.md docs
git commit -m "chore: initial commit with spec and plan for the worktree infrastructure"
```

- [ ] **Step 5: Convert the checkout into a bare repository + `main` worktree**

```bash
cd /home/work/auth-gate
mv .git .bare
git --git-dir=.bare config core.bare true
echo "gitdir: ./.bare" > .git
git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
git worktree add main main
rm -rf .gitignore LICENSE README.md docs      # now live inside main/
ln -s ../local main/local
```

- [ ] **Step 6: Verify**

Run: `git worktree list && ls -la /home/work/auth-gate && ls -l main/local && git -C main status --short`
Expected: two lines (`.bare (bare)` and `main <sha> [main]`); root contains only `.bare`, `.git`, `local`, `main`; `main/local -> ../local`; status empty.

- [ ] **Step 7 (HUMAN): push `main`**

```bash
git -C /home/work/auth-gate/main push -u origin main
```

---

### Task 2: Verify the Claude project key and expose it at `.bare/claude`

**Files:** none in the repo; a note for Task 14's docs.

- [ ] **Step 1: Ask Claude where its memory is, from inside `main/`**

Run: `cd /home/work/auth-gate/main && claude -p 'Reply with only the absolute path of your persistent memory directory as stated in your instructions.'`
Expected: `/home/ongdev2k2/.claude/projects/-home-work-auth-gate--bare/memory/`. Record the actual value; if it differs, the key is whatever was printed and `bare_key` in Task 5 must compute that value instead of `key_of "$root/.bare"`.

- [ ] **Step 2: Create the convenience symlink**

```bash
key="-home-work-auth-gate--bare"     # the value observed in Step 1
mkdir -p "$HOME/.claude/projects/$key"
ln -sfn "$HOME/.claude/projects/$key" /home/work/auth-gate/.bare/claude
```

- [ ] **Step 3: Verify**

Run: `ls -l /home/work/auth-gate/.bare/claude && ls /home/work/auth-gate/.bare/claude/`
Expected: symlink to the key directory; the directory lists `memory` (created by Step 1 or empty).

- [ ] **Step 4 (HUMAN, after the planning session has ended): move the pre-conversion memory to the new key**

The planning session ran at the workspace root, so its memories sit under the old key. Copy them only if the key observed in Step 1 is different from `-home-work-auth-gate`:

```bash
old="$HOME/.claude/projects/-home-work-auth-gate/memory"
new="$HOME/.claude/projects/-home-work-auth-gate--bare/memory"     # the key from Step 1
mkdir -p "$new" && cp -rn "$old/." "$new/" && cat "$new/MEMORY.md"
```
Expected: `MEMORY.md` lists the round-1 status and user-prefs entries. Record the outcome for Task 14 (`docs/infra/04-claude-sessions.md`, "Verified on").

---

### Task 3: Orchestrator orphan branch with the first assignment

**Files:**
- Create (on branch `orchestrator`, in `/home/work/auth-gate/orchestrator/`): `CLAUDE.md`, `BOARD.md`, `handoffs/feat-infra-tooling/00-brief.md`.

**Interfaces:**
- Produces: `BOARD.md` table with columns `Worktree | Branch | Task | Status | Last handoff | Notes`, status vocabulary `planned, active, review, merged, abandoned`; brief file format `# Brief: <branch>` then sections `## Goal`, `## Constraints`, `## Definition of done`, `## References`.

- [ ] **Step 1: Create the worktree and make the branch an orphan**

```bash
cd /home/work/auth-gate
git worktree add --detach orchestrator main
cd orchestrator
git checkout --orphan orchestrator
git rm -rfq .
ln -s ../local local
```

- [ ] **Step 2: Write `CLAUDE.md` for the orchestrator role**

Content covers every bullet of spec section 8 "orchestrator/CLAUDE.md", in this order: role (coordinate, never write product code); `BOARD.md` is the single source of truth; every worktree action is a fenced bash block the human pastes, scripts at `../main/tools/wt/`; one feature per worktree; assignment procedure (add row `planned`, write `handoffs/<dir>/00-brief.md`, commit, print `new.sh` + `cd ... && claude`); merge procedure (last handoff says done and row is `review`, then print `merge.sh`, then `rm.sh`, then set row `merged`); handoff procedure for itself under `handoffs/orchestrator/`; commit on this branch after every BOARD change with a `chore(board): ...` message; chat replies in Vietnamese, files in English.

- [ ] **Step 3: Write `BOARD.md` with one row**

```markdown
# Board

| Worktree | Branch | Task | Status | Last handoff | Notes |
|---|---|---|---|---|---|
| feat-infra-tooling | feat/infra-tooling | Worktree scripts, hooks, pnpm skeleton, CI, docs (round 1) | planned | 00-brief | plan: docs/superpowers/plans/2026-10-18-worktree-infrastructure.md |

Status values: planned, active, review, merged, abandoned.
```

- [ ] **Step 4: Write `handoffs/feat-infra-tooling/00-brief.md`**

Goal: implement Tasks 4 to 13 of the plan. Constraints: Global Constraints of the plan. Definition of done: spec section 12. References: spec and plan paths under `../main/docs/superpowers/`.

- [ ] **Step 5: Commit and verify**

```bash
git add -A
git commit -m "chore: bootstrap orchestrator branch with board and first brief"
git log --oneline | wc -l      # expected: 1
git -C /home/work/auth-gate worktree list
```
Expected: `orchestrator <sha> [orchestrator]` listed; the branch has exactly one commit and no parent.

- [ ] **Step 6 (HUMAN): push the branch**

```bash
git -C /home/work/auth-gate/orchestrator push -u origin orchestrator
```

---

### Task 4: Feature worktree, `lib.sh`, and the e2e test harness

**Files:**
- Create: `tools/wt/lib.sh`, `tools/wt/test/e2e.sh`.

**Interfaces:**
- Produces (`lib.sh`, sourced): `BASE_BRANCH=main`; `CLAUDE_PROJECTS_DIR` (env default `$HOME/.claude/projects`); `die <msg>` (red `error:`, exit 1); `warn <msg>` (yellow); `step <msg>` (cyan `==>`); `wt_root` (echo the nearest ancestor of `$PWD` or of the script that contains `.bare`, else die); `dir_of <branch>` (`/` → `-`); `key_of <abs-path>` (every character outside `A-Za-z0-9` → `-`); `bare_key` (`key_of "$root/.bare"`).
- Produces (`e2e.sh`): a self-contained test that seeds a throwaway remote with the `tools/wt` under test, then exercises the lifecycle; prints TAP-like `ok - ` / `not ok - ` lines; exits 1 if any `not ok`.

- [ ] **Step 1: Create the worktree by hand (bootstrap exception, `new.sh` does not exist yet)**

```bash
cd /home/work/auth-gate
git worktree add --no-track -b feat/infra-tooling feat-infra-tooling main
ln -s ../local feat-infra-tooling/local
cd feat-infra-tooling
```
All following tasks run inside `/home/work/auth-gate/feat-infra-tooling`.

- [ ] **Step 2: Write the failing harness `tools/wt/test/e2e.sh`**

Harness structure: `tmp=$(mktemp -d)` with `trap 'rm -rf "$tmp"' EXIT`; export `CLAUDE_PROJECTS_DIR="$tmp/claude-projects"`, `WT_GATE=true`, `GIT_AUTHOR_NAME=e2e`, `GIT_AUTHOR_EMAIL=e2e@example.com`, same for committer; helpers `ok`, `nok`, `assert <msg> <cmd...>`; seed function that creates `$tmp/remote.git` (bare) and pushes a `main` containing `tools/wt/` (copied from the repo under test), `package.json` (`{"name":"e2e","private":true}`), `docs/infra/templates/orchestrator/` and `.claude/` (copied when present); end with `[ "$fail" = 0 ]`. First assertions:

```bash
# shellcheck source=../lib.sh
. "$wt/lib.sh"
assert "dir_of maps slash to dash" test "$(dir_of feat/x)" = "feat-x"
assert "key_of replaces non-alphanumerics" test "$(key_of /a/b.c)" = "-a-b-c"
```

- [ ] **Step 3: Run it, expect failure**

Run: `bash tools/wt/test/e2e.sh`
Expected: fails with `lib.sh: No such file`.

- [ ] **Step 4: Implement `tools/wt/lib.sh` with the interfaces above**

`wt_root` walks up from `$PWD` first, then from the sourcing script's directory, so it works both from inside a worktree and through a `local/` symlink.

- [ ] **Step 5: Run the first two assertions**

Run: `bash tools/wt/test/e2e.sh`
Expected: `ok - dir_of maps slash to dash`, `ok - key_of replaces non-alphanumerics`, exit 0.

- [ ] **Step 6: Commit**

```bash
git add tools/wt/lib.sh tools/wt/test/e2e.sh
git commit -m "feat(wt): shared library and e2e harness for worktree scripts"
```

---

### Task 5: `init.sh` and the orchestrator templates

**Files:**
- Create: `tools/wt/init.sh`, `docs/infra/templates/orchestrator/CLAUDE.md`, `docs/infra/templates/orchestrator/BOARD.md`, `docs/infra/templates/orchestrator/handoffs/.gitkeep`.
- Modify: `tools/wt/test/e2e.sh`.

**Interfaces:**
- Consumes: `lib.sh`.
- Produces: `init.sh <repo-url> [<dir>]` creates `<dir>/{.bare,.git,local,main,orchestrator}` and `.bare/claude`; exit 0 on success. Templates: the Task 3 files with the `feat-infra-tooling` row and brief removed (BOARD has the header and the status line only).

- [ ] **Step 1: Add e2e assertions**

```bash
"$wt/init.sh" "$tmp/remote.git" "$tmp/ws"
assert "init: bare repo"           test -f "$tmp/ws/.bare/HEAD"
assert "init: wt_root from a subfolder" test "$(cd "$tmp/ws/main" && wt_root)" = "$tmp/ws"
assert "init: .git pointer"        grep -qx 'gitdir: ./.bare' "$tmp/ws/.git"
assert "init: main worktree"       test "$(git -C "$tmp/ws/main" branch --show-current)" = main
assert "init: orchestrator orphan" test "$(git -C "$tmp/ws/orchestrator" rev-list --count HEAD)" = 1
assert "init: orchestrator board"  test -f "$tmp/ws/orchestrator/BOARD.md"
assert "init: local symlinks"      test -L "$tmp/ws/main/local" -a -L "$tmp/ws/orchestrator/local"
assert "init: .bare/claude link"   test "$(readlink "$tmp/ws/.bare/claude")" = "$CLAUDE_PROJECTS_DIR/$(cd "$tmp/ws" && bare_key)"
assert "init: idempotent refusal"  ! "$wt/init.sh" "$tmp/remote.git" "$tmp/ws" 2>/dev/null
```

- [ ] **Step 2: Run, expect `not ok` lines for init**

- [ ] **Step 3: Write the templates, then `tools/wt/init.sh`**

Order inside `init.sh`: refuse if `<dir>/.bare` exists; `git clone --bare`; `.git` pointer; fetch refspec + `git fetch origin`; `git worktree add main main` (die if no `main`); orchestrator: existing branch → `git worktree add orchestrator orchestrator`, otherwise detached worktree + `checkout --orphan` + `git rm -rfq .` + copy `main/docs/infra/templates/orchestrator/` and `main/.claude/` (when present) + commit `chore: bootstrap orchestrator branch`; `mkdir -p local`; the two `local` symlinks; `mkdir -p "$CLAUDE_PROJECTS_DIR/$(bare_key)"` and the `.bare/claude` symlink; print next steps (push orchestrator if new, `cd main && pnpm install`).

- [ ] **Step 4: Run e2e**

Run: `bash tools/wt/test/e2e.sh`
Expected: all `init:` lines `ok`, plus the `wt_root` assertion from Task 4.

- [ ] **Step 5: Commit**

```bash
git add tools/wt/init.sh docs/infra/templates tools/wt/test/e2e.sh
git commit -m "feat(wt): init.sh bootstraps a bare workspace with main and orchestrator"
```

---

### Task 6: `new.sh`

**Files:**
- Create: `tools/wt/new.sh`. Modify: `tools/wt/test/e2e.sh`.

**Interfaces:**
- Consumes: `lib.sh`; `claude-link.sh` from Task 9 (call it only if the file exists, with `warn` otherwise, until Task 9 lands; after Task 9 the call is unconditional).
- Produces: `new.sh <branch> [--from <branch>] [--no-install] [--push]`; on success prints the final line `cd <root>/<dir> && claude`.

- [ ] **Step 1: Add e2e assertions**

```bash
(cd "$tmp/ws/main" && ./tools/wt/new.sh feat/x --no-install)
assert "new: worktree on branch"    test "$(git -C "$tmp/ws/feat-x" branch --show-current)" = feat/x
assert "new: forked from main"      test "$(git -C "$tmp/ws/feat-x" rev-parse HEAD)" = "$(git -C "$tmp/ws/main" rev-parse HEAD)"
assert "new: local symlink"         test -L "$tmp/ws/feat-x/local"
assert "new: no upstream yet"       ! git -C "$tmp/ws/feat-x" rev-parse --abbrev-ref '@{u}' >/dev/null 2>&1
before="$(git -C "$tmp/ws/main" worktree list | wc -l)"
assert "new: refuses a branch already checked out" ! (cd "$tmp/ws/main" && ./tools/wt/new.sh feat/x --no-install) 2>/dev/null
assert "new: nothing half-created"  test "$(git -C "$tmp/ws/main" worktree list | wc -l)" = "$before" -a -z "$(ls -A "$CLAUDE_PROJECTS_DIR" 2>/dev/null | grep -v -- "$(cd "$tmp/ws" && bare_key)" | grep -v feat-x || true)"
(cd "$tmp/ws/main" && ./tools/wt/new.sh feat/pushed --no-install --push)
assert "new --push: upstream set"   test "$(git -C "$tmp/ws/feat-pushed" rev-parse --abbrev-ref '@{u}')" = origin/feat/pushed
```

- [ ] **Step 2: Run, expect `not ok`**

- [ ] **Step 3: Implement `new.sh`**

Preconditions before any side effect: root found, `main/` exists, `<dir>` does not exist, and for a brand-new branch `origin/<from>` exists (die with "push it first" otherwise). Three branch cases as in the reference script (local exists, remote-only, new with `--no-track`). Then `ln -s ../local`, `pnpm install` unless `--no-install` (skipped with `warn` when `<dir>/package.json` is missing), `claude-link.sh "$root/$dir"`, optional `git push --set-upstream origin <branch>`.

- [ ] **Step 4: Run e2e, expect all `new:` lines `ok`**

- [ ] **Step 5: Commit** — `feat(wt): new.sh creates a ready-to-code worktree`

---

### Task 7: `merge.sh`

**Files:**
- Create: `tools/wt/merge.sh`. Modify: `tools/wt/test/e2e.sh`.

**Interfaces:**
- Produces: `merge.sh <branch> [--no-push]`; exit 1 before touching anything if either worktree is dirty; runs `$WT_GATE` inside the feature worktree; fast-forwards `main`; pushes `main` unless `--no-push`.

- [ ] **Step 1: Add e2e assertions**

```bash
echo change > "$tmp/ws/feat-x/CHANGE"; git -C "$tmp/ws/feat-x" add CHANGE; git -C "$tmp/ws/feat-x" commit -qm "feat: change"
echo dirty > "$tmp/ws/feat-x/DIRTY"
assert "merge: refuses dirty worktree" ! (cd "$tmp/ws/main" && ./tools/wt/merge.sh feat/x) 2>/dev/null
assert "merge: main untouched after refusal" test "$(git -C "$tmp/ws/main" rev-list --count HEAD)" = 1
rm "$tmp/ws/feat-x/DIRTY"
assert "merge: gate failure stops the merge" ! (cd "$tmp/ws/main" && WT_GATE=false ./tools/wt/merge.sh feat/x) 2>/dev/null
(cd "$tmp/ws/main" && ./tools/wt/merge.sh feat/x)
assert "merge: main fast-forwarded"   test "$(git -C "$tmp/ws/main" rev-parse HEAD)" = "$(git -C "$tmp/ws/feat-x" rev-parse HEAD)"
assert "merge: main pushed"           test "$(git -C "$tmp/ws/main" rev-parse origin/main)" = "$(git -C "$tmp/ws/main" rev-parse HEAD)"
assert "merge: history is linear"     test "$(git -C "$tmp/ws/main" rev-list --merges --count HEAD)" = 0
```

- [ ] **Step 2: Run, expect `not ok`**

- [ ] **Step 3: Implement `merge.sh`**

Sequence: resolve `<dir>`; die if `git -C <dir> status --porcelain` or `git -C main status --porcelain` is non-empty; `git fetch origin` when `origin` exists; `git -C main pull --ff-only` when `main` has an upstream; `git -C <dir> rebase main`; `(cd <dir> && eval "$WT_GATE")`; `git -C main merge --ff-only <branch>`; `git -C main push origin main` unless `--no-push`. Each phase printed with `step`.

- [ ] **Step 4: Run e2e, expect all `merge:` lines `ok`**

- [ ] **Step 5: Commit** — `feat(wt): merge.sh rebases, gates, and fast-forwards main`

---

### Task 8: `rm.sh` and `status.sh`

**Files:**
- Create: `tools/wt/rm.sh`, `tools/wt/status.sh`. Modify: `tools/wt/test/e2e.sh`.

**Interfaces:**
- Produces: `rm.sh <branch> [--force]`; `status.sh` prints `git worktree list`, one `  <branch>: ahead N, behind M (vs main)` line per non-main branch, the BOARD table lines from `orchestrator/BOARD.md`, and `warn: orchestrator/.claude differs from main/.claude` when `diff -rq` finds a difference.

- [ ] **Step 1: Add e2e assertions**

```bash
echo p > "$tmp/ws/feat-pushed/P"; git -C "$tmp/ws/feat-pushed" add P; git -C "$tmp/ws/feat-pushed" commit -qm "feat: pushed-only change"
out="$(cd "$tmp/ws/main" && ./tools/wt/status.sh)"
assert "status: lists worktrees"  grep -q 'feat-x' <<<"$out"
assert "status: shows board"      grep -q '| Worktree |' <<<"$out"
assert "rm: refuses unmerged without --force" ! (cd "$tmp/ws/main" && ./tools/wt/rm.sh feat/pushed) 2>/dev/null
assert "rm: unmerged still there" test -d "$tmp/ws/feat-pushed"
(cd "$tmp/ws/main" && ./tools/wt/rm.sh feat/x)
assert "rm: worktree gone"        test ! -e "$tmp/ws/feat-x"
assert "rm: local branch gone"    ! git -C "$tmp/ws/main" show-ref --verify --quiet refs/heads/feat/x
assert "rm: claude link gone"     test ! -e "$CLAUDE_PROJECTS_DIR/$(key_of "$tmp/ws/feat-x")"
rm -rf "$tmp/ws/feat-pushed"                       # deleted by hand, stale metadata remains
(cd "$tmp/ws/main" && ./tools/wt/rm.sh feat/pushed --force)
assert "rm --force: stale worktree pruned" ! git -C "$tmp/ws/main" worktree list | grep -q feat-pushed
assert "rm --force: remote branch gone"    ! git -C "$tmp/ws/main" ls-remote --exit-code --heads origin feat/pushed >/dev/null 2>&1
```

- [ ] **Step 2: Run, expect `not ok`**

- [ ] **Step 3: Implement `rm.sh`**

Merged check: `git -C main branch --merged main --format='%(refname:short)' | grep -qx "$branch"`; without `--force` die when not merged. Then: if `<dir>` exists `git worktree remove [--force] <dir>`, else `git worktree prune`; `git branch -d` (or `-D` with `--force`); `git push origin --delete <branch>` only when `origin/<branch>` exists; remove `$CLAUDE_PROJECTS_DIR/$(key_of "$root/$dir")` only when it is a symlink (a real directory gets `warn` and stays).

- [ ] **Step 4: Implement `status.sh`** per the Produces line; ahead/behind from `git rev-list --left-right --count main...<branch>`.

- [ ] **Step 5: Run e2e, expect all `rm:` and `status:` lines `ok`**

- [ ] **Step 6: Commit** — `feat(wt): rm.sh and status.sh`

---

### Task 9: `claude-link.sh` and the transcript smoke test

**Files:**
- Create: `tools/wt/claude-link.sh`. Modify: `tools/wt/new.sh` (make the call unconditional), `tools/wt/test/e2e.sh`.

**Interfaces:**
- Produces: `claude-link.sh <worktree-dir>` makes `$CLAUDE_PROJECTS_DIR/$(key_of <abs-dir>)` a symlink to `$CLAUDE_PROJECTS_DIR/$(bare_key)`; idempotent; a real non-empty directory is renamed to `<key>.bak-<yyyymmdd-hhmmss>` first; a real empty directory is removed.

- [ ] **Step 1: Add e2e assertions (insert before the `rm:` block so `feat-x` still exists)**

```bash
link="$CLAUDE_PROJECTS_DIR/$(key_of "$tmp/ws/feat-x")"; target="$CLAUDE_PROJECTS_DIR/$(cd "$tmp/ws" && bare_key)"
assert "link: created by new.sh"     test "$(readlink "$link")" = "$target"
(cd "$tmp/ws/main" && ./tools/wt/claude-link.sh "$tmp/ws/feat-x")
assert "link: idempotent"            test "$(readlink "$link")" = "$target"
rm "$link"; mkdir -p "$link"; echo x > "$link/old.jsonl"
(cd "$tmp/ws/main" && ./tools/wt/claude-link.sh "$tmp/ws/feat-x")
assert "link: real dir backed up"    ls -d "$link".bak-* >/dev/null
assert "link: backup keeps content"  test -f "$link".bak-*/old.jsonl
assert "link: now a symlink"         test "$(readlink "$link")" = "$target"
```

- [ ] **Step 2: Run, expect `not ok`**

- [ ] **Step 3: Implement `claude-link.sh`; make `new.sh` call it unconditionally**

- [ ] **Step 4: Run e2e, expect everything `ok`**

- [ ] **Step 5: Link this worktree for real and run the transcript smoke test**

```bash
./tools/wt/claude-link.sh /home/work/auth-gate/feat-infra-tooling
claude -p 'Reply with the single word ok.'
ls -t /home/work/auth-gate/.bare/claude/*.jsonl | head -1
```
Expected: a `.jsonl` modified just now under `.bare/claude/`. If instead it appears under `~/.claude/projects/-home-work-auth-gate-feat-infra-tooling/` as a real directory (Claude replaced the symlink), the result is "symlink not honoured": remove the `claude-link.sh` call from `new.sh`, keep the script for manual use, and record the outcome. Either outcome is written into `docs/infra/04-claude-sessions.md` in Task 14 under "Verified on 2026-10-18".

- [ ] **Step 6: Commit** — `feat(wt): claude-link.sh consolidates Claude data under the .bare key`

---

### Task 10: Hooks, statusline, `/handoff`, settings

**Files:**
- Create: `.claude/settings.json`, `.claude/statusline.sh`, `.claude/hooks/session-start.sh`, `.claude/hooks/context-guard.sh`, `.claude/commands/handoff.md`, `tools/wt/test/hooks.sh`.

**Interfaces:**
- `statusline.sh`: stdin JSON (`session_id`, `workspace.current_dir`, `context_window.used_percentage`, `cost.total_cost_usd`); stdout `<branch> | ctx <N>% | $<cost to 2 dp>`; writes integer `N` to `$CLAUDE_CTX_DIR/<session_id>` (`used_percentage // 0`, floored).
- `context-guard.sh`: constants `WARN=70`, `STOP=85` at the top; stdin JSON with `session_id`; reads the same file; prints nothing below `WARN`; from `WARN` prints `[context-guard] Context is at N%. Finish the current step, then run /handoff and tell the user to start a new session.`; from `STOP` prints `[context-guard] Context is at N%. STOP. Write the handoff now with /handoff and print the command to start a new session. Do not start new work.`; always exit 0.
- `session-start.sh`: stdin JSON with `cwd`; `orch="$cwd/../orchestrator"`; when the current branch is `orchestrator` prints all of `BOARD.md`; otherwise prints the BOARD row matching `| <basename cwd> |` or `No BOARD row for <dir>. Ask the orchestrator session for an assignment.`, then the lexically last `*.md` under `$orch/handoffs/<dir>/` capped at 120 lines (or `No handoff yet.`), then `More: docs/infra/00-overview.md`. Missing orchestrator folder prints one line `Orchestrator worktree not found at ../orchestrator.` and exits 0.
- `handoff.md`: the `/handoff` command text per spec section 7 (write `../orchestrator/handoffs/<dir>/<yyyy-mm-dd-hhmm>.md` in the five-section format, update the BOARD row's Status and Last handoff, `git -C ../orchestrator add -A && git -C ../orchestrator commit -m "chore(board): handoff <branch> <timestamp>"`, end the reply with `cd <abs worktree> && claude`).

- [ ] **Step 1: Write the failing `tools/wt/test/hooks.sh`**

Fixture in `$tmp`: `ws/orchestrator/BOARD.md` with a `feat-x` row, `ws/orchestrator/handoffs/feat-x/00-brief.md` containing `BRIEF-MARKER`, `ws/feat-x` as `git init -q && git checkout -q -b feat/x`, `ws/feat-none` as a repo on `feat/none`; `export CLAUDE_CTX_DIR="$tmp/ctx"`. Assertions:

```bash
out="$(printf '{"session_id":"t1","workspace":{"current_dir":"%s"},"context_window":{"used_percentage":42.7},"cost":{"total_cost_usd":0.123}}' "$tmp/ws/feat-x" | bash "$dotclaude/statusline.sh")"
assert "statusline: text"        test "$out" = 'feat/x | ctx 42% | $0.12'
assert "statusline: file"        test "$(cat "$tmp/ctx/t1")" = 42
assert "guard: silent below 70"  test -z "$(echo '{"session_id":"t1"}' | bash "$dotclaude/hooks/context-guard.sh")"
echo 70 > "$tmp/ctx/t1"; assert "guard: warns at 70"  grep -q 'run /handoff' <<<"$(echo '{"session_id":"t1"}' | bash "$dotclaude/hooks/context-guard.sh")"
echo 85 > "$tmp/ctx/t1"; assert "guard: stops at 85"  grep -q 'STOP' <<<"$(echo '{"session_id":"t1"}' | bash "$dotclaude/hooks/context-guard.sh")"
assert "guard: no file is silent" test -z "$(echo '{"session_id":"nope"}' | bash "$dotclaude/hooks/context-guard.sh")"
out="$(printf '{"cwd":"%s"}' "$tmp/ws/feat-x" | bash "$dotclaude/hooks/session-start.sh")"
assert "start: board row"        grep -q '| feat-x |' <<<"$out"
assert "start: latest handoff"   grep -q 'BRIEF-MARKER' <<<"$out"
out="$(printf '{"cwd":"%s"}' "$tmp/ws/feat-none" | bash "$dotclaude/hooks/session-start.sh")"
assert "start: no row message"   grep -q 'No BOARD row for feat-none' <<<"$out"
assert "start: no handoff message" grep -q 'No handoff yet' <<<"$out"
assert "start: missing orchestrator exits 0" sh -c "printf '{\"cwd\":\"$tmp/nowhere\"}' | bash '$dotclaude/hooks/session-start.sh' >/dev/null"
```

- [ ] **Step 2: Run, expect failures** — `bash tools/wt/test/hooks.sh`

- [ ] **Step 3: Implement the three scripts with `jq -r`, then `settings.json` and `handoff.md`**

`settings.json`:

```json
{
  "statusLine": { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/statusline.sh" },
  "hooks": {
    "SessionStart": [ { "matcher": "startup|resume|clear|compact",
      "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/hooks/session-start.sh" } ] } ],
    "UserPromptSubmit": [ { "hooks": [ { "type": "command", "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/hooks/context-guard.sh" } ] } ]
  }
}
```

- [ ] **Step 4: Run hooks test, expect all `ok`**

- [ ] **Step 5: Verify SessionStart injection end to end**

Run (in `feat-infra-tooling`): `claude -p 'In one line: what does the BOARD say your assignment is?'`
Expected: a line mentioning worktree scripts / round 1. If the hook did not fire, check `claude --debug` output for the hook command and fall back to the relative form `bash .claude/hooks/session-start.sh` in `settings.json`.

- [ ] **Step 6: Commit** — `feat(claude): session-start and context-guard hooks, statusline, /handoff`

---

### Task 11: pnpm workspace skeleton with a real gate

**Files:**
- Create: `package.json`, `pnpm-workspace.yaml`, `.npmrc`, `packages/.gitkeep`, `apps/.gitkeep`, `pnpm-lock.yaml` (generated).

**Interfaces:**
- Produces: `pnpm run check` = `lint` (`shellcheck tools/wt/*.sh tools/wt/test/*.sh .claude/statusline.sh .claude/hooks/*.sh`), then `typecheck` (`pnpm -r --if-present run typecheck`), then `test` (`bash tools/wt/test/hooks.sh && bash tools/wt/test/e2e.sh && pnpm -r --if-present run test`). `packageManager` is `pnpm@10.13.1`, `engines.node` is `>=22`. Dev dependency: `shellcheck` (npm wrapper, so CI has the binary).

- [ ] **Step 1: Write `package.json`, `pnpm-workspace.yaml` (`packages: ["packages/*", "apps/*"]`), `.npmrc` (one comment line: `# pnpm settings for auth-gate; defaults apply`)**

- [ ] **Step 2: Install and run the gate**

Run: `pnpm add -Dw shellcheck && pnpm run check`
Expected: shellcheck reports nothing, typecheck prints nothing, both test scripts end with all `ok`, exit 0. Fix any shellcheck finding in the scripts here.

- [ ] **Step 3: Verify the store is shared**

Run: `pnpm store path && find node_modules -type f -links +1 | head -1`
Expected: the store path from Task 0; at least one hard-linked file.

- [ ] **Step 4: Commit** — `chore: pnpm workspace skeleton with lint, typecheck and test gate`

---

### Task 12: CI

**Files:**
- Create: `.github/workflows/ci.yml`.

- [ ] **Step 1: Write the workflow**

Trigger on `push` to `main` and on `pull_request`; one job `check` on `ubuntu-latest`: `actions/checkout@v4`, `pnpm/action-setup@v4` (reads `packageManager`), `actions/setup-node@v4` with `node-version: 22` and `cache: pnpm`, `pnpm install --frozen-lockfile`, `pnpm run check`.

- [ ] **Step 2: Validate locally**

Run: `pnpm install --frozen-lockfile && pnpm run check`
Expected: exit 0 (the e2e test sets its own git identity, so it runs on a clean runner).

- [ ] **Step 3: Commit** — `ci: run the gate on push and pull request`

---

### Task 13: `CLAUDE.md`, `README.md`, `CONTRIBUTING.md`, ADRs

**Files:**
- Create: `CLAUDE.md`, `CONTRIBUTING.md`, `docs/adr/0001-bare-repo-and-worktrees.md`, `docs/adr/0002-orchestrator-is-an-orphan-branch.md`, `docs/adr/0003-handoff-not-transcript.md`.
- Modify: `README.md`.

- [ ] **Step 1: Write `CLAUDE.md`** with exactly the spec section 8 "main/CLAUDE.md" bullets, under 40 lines.

- [ ] **Step 2: Write the three ADRs** in the format `# N. Title` / `Date` / `Status: accepted` / `## Context` / `## Decision` / `## Consequences`, each under 60 lines, reasons taken from spec sections 3, 4 and 7.

- [ ] **Step 3: Write `README.md`** sections: what auth-gate will be (one paragraph, round 2 pointer), repository layout (the tree from spec section 4), developing (Task 0 pnpm note, `pnpm install`, `pnpm run check`), the worktree workflow in six lines with a link to `docs/infra/`, license.

- [ ] **Step 4: Write `CONTRIBUTING.md`**: Conventional Commits, one feature per branch, `pnpm run check` must pass, how to run the e2e test alone.

- [ ] **Step 5: Verify and commit**

Run: `pnpm run check` — Expected: exit 0.
Commit: `docs: CLAUDE.md, README, CONTRIBUTING and the first three ADRs`

---

### Task 14: `docs/infra/` (the reproducible design)

**Files:**
- Create: `docs/infra/00-overview.md`, `01-workspace-layout.md`, `02-worktree-lifecycle.md`, `03-orchestrator.md`, `04-claude-sessions.md`, `05-pnpm.md`, `06-bootstrap-new-project.md`.

Each file opens with a `## Why` section, then `## How`. Written for a human first.

- [ ] **Step 1: `00-overview.md`** — the problem (parallel features, Claude sessions that forget), the picture (tree from spec section 3), the reading order, and a one-paragraph summary of each other file.

- [ ] **Step 2: `01-workspace-layout.md`** — spec section 3 rules, what each branch contains (section 4), `.gitignore` and `local/` (section 10), the `.bare/claude` symlink.

- [ ] **Step 3: `02-worktree-lifecycle.md`** — spec section 5 as a numbered walkthrough with the exact commands, then a table of all scripts and flags (section 6), then "by hand" equivalents for each script (so the reader understands what the script does), then recovery: stale worktree (`git worktree prune`), detached HEAD, a rebase conflict in `merge.sh`.

- [ ] **Step 4: `03-orchestrator.md`** — the role, `BOARD.md` format and status vocabulary, the brief and handoff formats, the commit rule, what the orchestrator must never do, and the orchestrator's own handoff.

- [ ] **Step 5: `04-claude-sessions.md`** — the three facts from spec section 7 with the doc URLs the research used (`code.claude.com/docs/en/memory`, `/sessions`, `/hooks`, `/statusline`), the design (memory default, transcript link, handoff), each hook's input and output, the context guard thresholds and how to change them, `/handoff`, and a `## Verified on 2026-10-18` section holding the Task 2 key result, the Task 9 transcript result, and the Task 10 injection result, verbatim.

- [ ] **Step 6: `05-pnpm.md`** — why pnpm for worktrees (store + hard links), the corepack signature failure and its fix, `packageManager`, how the gate is composed.

- [ ] **Step 7: `06-bootstrap-new-project.md`** — the checklist Claude follows for a new project: prerequisites (git ≥ 2.39, pnpm, jq, shellcheck, Claude Code), copy `tools/wt/`, `.claude/`, `docs/infra/`, `CLAUDE.md` into the new repo's `main`, run `init.sh <url>`, create the first brief, start the orchestrator session. Followed by the "convert an existing checkout" variant (spec section 11 steps 1 to 6 generalised). Ends with the spec section 12 checklist as the acceptance test.

- [ ] **Step 8: Fresh-eyes test**

Run: `claude -p 'Read docs/infra/*.md only. In ten lines, explain the workspace layout, how a feature goes from assignment to removal, and how a new session learns where the last one stopped. Name any question the docs leave open.'`
Expected: a correct explanation and no open question that the docs should have answered. Fix the docs for any gap it names, then re-run once.

- [ ] **Step 9: Commit** — `docs(infra): reproducible design for the worktree workspace`

---

### Task 15: Sync `.claude/` to the orchestrator branch and mark the row `review`

**Files:**
- Create on branch `orchestrator`: `.claude/` (copy of `main`'s), `handoffs/feat-infra-tooling/<yyyy-mm-dd-hhmm>.md`. Modify: `BOARD.md`.

- [ ] **Step 1: Copy and verify no drift**

```bash
cp -r /home/work/auth-gate/feat-infra-tooling/.claude /home/work/auth-gate/orchestrator/.claude
diff -rq /home/work/auth-gate/feat-infra-tooling/.claude /home/work/auth-gate/orchestrator/.claude && echo same
```
Expected: `same`.

- [ ] **Step 2: Write the handoff in the five-section format** (Done: Tasks 4 to 14; In progress: none; Remaining: Tasks 16 to 18; Decisions: the Task 9 transcript outcome; Next command: `cd /home/work/auth-gate/orchestrator && claude`). Set the BOARD row to `review` with the handoff timestamp.

- [ ] **Step 3: Commit on `orchestrator`**

```bash
git -C /home/work/auth-gate/orchestrator add -A
git -C /home/work/auth-gate/orchestrator commit -m "chore(board): feat/infra-tooling ready for review; add .claude"
```

---

### Task 16: Merge the tooling branch with its own `merge.sh` (HUMAN)

- [ ] **Step 1 (HUMAN): push the feature branch, then merge**

```bash
cd /home/work/auth-gate/feat-infra-tooling
git push -u origin feat/infra-tooling
./tools/wt/merge.sh feat/infra-tooling
```
Expected: `==>` lines for fetch, pull, rebase, gate (`pnpm run check` all `ok`), fast-forward, push; `git -C ../main log --oneline | head -3` shows the tooling commits on `main`.

- [ ] **Step 2 (HUMAN): remove the worktree**

```bash
cd /home/work/auth-gate/main
./tools/wt/rm.sh feat/infra-tooling
./tools/wt/status.sh
```
Expected: `feat-infra-tooling` absent from the worktree list; no `warn:` about `.claude` drift; BOARD row still `review` (Step 3 fixes it).

- [ ] **Step 3: Orchestrator bookkeeping** — set the row to `merged`, commit `chore(board): feat/infra-tooling merged`, **HUMAN** pushes `orchestrator`.

---

### Task 17: Interactive verification (HUMAN, with Claude recording results)

- [ ] **Step 1 (HUMAN): create a throwaway feature through the real lifecycle**

```bash
cd /home/work/auth-gate/orchestrator && claude
```
Say: "assign a throwaway task `feat/smoke`: add one line to README". Expected: a BOARD row, a brief, and a pasted-ready `new.sh` block. Run it, then `cd ../feat-smoke && claude`.

- [ ] **Step 2 (HUMAN): check the four signals**

1. First reply knows the assignment when asked "what is your assignment?".
2. The status bar shows `feat/smoke | ctx N% | $...`, and `cat /tmp/claude-ctx/$(ls -t /tmp/claude-ctx | head -1)` prints a number.
3. Edit `.claude/hooks/context-guard.sh` to `WARN=0`, send any prompt: the reply mentions running `/handoff`. Restore `WARN=70`.
4. Run `/handoff`: a file appears under `../orchestrator/handoffs/feat-smoke/`, the BOARD row changes, `git -C ../orchestrator log -1` shows the commit, and the reply ends with a `cd ... && claude` line.

- [ ] **Step 3 (HUMAN): merge and remove `feat/smoke` from the orchestrator session**, confirming it prints `merge.sh` then `rm.sh` and moves the row to `merged`.

- [ ] **Step 4: Record** the four outcomes in `docs/infra/04-claude-sessions.md` "Verified on" (on a `fix/docs-verified` branch through the lifecycle, or directly on `main` if the human says so).

---

### Task 18: Reproduce the workspace from GitHub with `init.sh` (HUMAN)

- [ ] **Step 1 (HUMAN)**

```bash
cd /tmp && rm -rf auth-gate-repro
/home/work/auth-gate/main/tools/wt/init.sh git@github.com:huy090202/auth-gate.git auth-gate-repro
ls -la auth-gate-repro && git -C auth-gate-repro worktree list
```
Expected: `.bare`, `.git`, `local`, `main`, `orchestrator`; three worktree lines; `orchestrator` on the existing remote branch (not a new orphan).

- [ ] **Step 2 (HUMAN): clean up**

```bash
rm -rf /tmp/auth-gate-repro
rm -rf ~/.claude/projects/-tmp-auth-gate-repro--bare
```

Round 1 is done when every item in spec section 12 has been observed. Round 2 (the auth-gate module) starts with a new brainstorming session in the orchestrator worktree.

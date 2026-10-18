# Worktree Infrastructure Design

**Date:** 2026-10-18
**Status:** draft, awaiting review
**Scope:** round 1 of the auth-gate project: the workspace layout, the worktree
tooling, the orchestrator, and the Claude session continuity mechanism. The
auth-gate module itself (PIN + WebAuthn on Lambda) is round 2 and gets its own
spec once this infrastructure is proven to work.

## 1. Goals

1. One repository, many parallel checkouts. Each feature is built in its own
   worktree folder, in its own editor window, in its own Claude Code session.
2. Claude never starts from zero. A new session in any worktree knows what the
   project is, what that worktree was assigned, and where the previous session
   stopped.
3. A single place, the orchestrator, decides which worktree does what, when a
   branch is merged, and when a worktree is removed. The orchestrator produces
   commands the human copies and pastes; it does not edit product code.
4. The human runs out of context before the work is lost: the session asks for
   a handoff and a fresh session when its context window is nearly full.
5. Dependencies do not multiply with worktrees: pnpm's shared store makes each
   worktree's `node_modules` a set of hard links.
6. Everything private stays in `local/`, which is never committed.
7. The design is reproducible: `docs/infra/` must let Claude rebuild this
   infrastructure for another project from the documents alone, and must let a
   human understand every piece.

## 2. Non-goals (round 1)

- Product code for the auth-gate module. Round 1 ends with an empty pnpm
  workspace that typechecks, lints and tests trivially.
- CI for product code. A minimal workflow that runs the gate on an empty
  workspace is in scope, so that the merge script has a real gate to call.
- Cross-machine sharing of Claude data. Memory and transcripts stay on this
  machine.

## 3. Workspace layout

```text
/home/work/auth-gate/                 # workspace root. NOT a checkout.
├── .bare/                            # the git repository (object db, refs, worktree metadata)
│   └── claude -> ~/.claude/projects/-home-work-auth-gate--bare/
├── .git                              # one line: "gitdir: ./.bare"
├── local/                            # the one real copy, gitignored, symlinked into every worktree
├── main/                             # worktree on branch main: product code, docs, tooling
├── orchestrator/                     # worktree on branch orchestrator: coordination state only
└── feat-<name>/                      # worktree on branch feat/<name>, forked from main
```

Rules that follow from the layout:

- **Directory name is the branch name with `/` replaced by `-`.** `feat/pin`
  lives in `feat-pin/`. No exceptions, so that scripts and humans can map one to
  the other without a lookup.
- **`main` is the only baseline.** Every feature forks from `origin/main` and
  merges back into `main` by fast-forward after a rebase.
- **`orchestrator` is an orphan branch.** It shares no history with `main` and
  is never merged into it. It is pushed to the remote so the coordination
  history is backed up.
- **`.bare` is the repository.** Deleting it destroys every worktree. The
  `claude` symlink inside it is for human navigation only; Claude Code does not
  read it. It points at the directory Claude Code already uses for this repo's
  memory (see section 7).
- **`local/` is a symlink in every worktree** pointing at `../local`. The name
  `local/` is in `.gitignore`, so the symlink never appears in `git status`.
- **The workspace root is not a place to run Claude.** Sessions run inside
  `main/`, `orchestrator/` or a `feat-*/` folder.

## 4. What lives on which branch

### `main`

```text
main/
├── .claude/
│   ├── settings.json         # statusline + hooks, committed
│   ├── statusline.sh         # shows branch and context %, records the % for the guard hook
│   ├── hooks/
│   │   ├── session-start.sh  # injects BOARD row + latest handoff into a new session
│   │   └── context-guard.sh  # injects a warning when context >= 70 %
│   └── commands/
│       └── handoff.md        # /handoff: write a handoff file, update BOARD, print next command
├── docs/
│   ├── infra/                # the reproducible design, written for humans first
│   ├── adr/                  # architecture decision records, one file per decision
│   └── superpowers/          # specs and plans
├── tools/wt/                 # worktree scripts (section 6)
├── packages/                 # pnpm workspace packages (empty in round 1)
├── apps/                     # deployable apps (empty in round 1)
├── .github/workflows/ci.yml  # lint + typecheck + test
├── .gitignore
├── .npmrc
├── package.json              # packageManager: pnpm@10.13.1, scripts: check, lint, typecheck, test
├── pnpm-workspace.yaml
├── CLAUDE.md                 # how a feature session works (section 8)
├── README.md
├── LICENSE                   # MIT
└── CONTRIBUTING.md
```

### `orchestrator`

```text
orchestrator/
├── .claude/                  # same files as main, copied; drift is checked by tools/wt/status.sh
├── CLAUDE.md                 # the orchestrator role (section 8)
├── BOARD.md                  # one table: who does what, in which state
└── handoffs/
    └── <branch-with-dashes>/ # e.g. feat-pin/
        ├── 00-brief.md       # the assignment, written by the orchestrator
        └── <yyyy-mm-dd-hhmm>.md   # one file per handoff, written by the feature session
```

Why an orphan branch and not a folder in `main`: coordination state changes
many times a day and has nothing to do with product history. Keeping it on its
own branch means `main` stays a clean, publishable open-source history, and the
orchestrator's own history is still versioned. The cost is one duplicated
`.claude/` directory, which `status.sh` checks for drift.

Why not `local/`: the state would have no history and could not be shown as
part of the reproducible design.

### `feat/*`

Forked from `main`, so each feature worktree carries `main`'s `.claude/`,
`CLAUDE.md` and tooling. Feature sessions read the orchestrator's files through
the relative path `../orchestrator/`.

## 5. Feature lifecycle

Every step is a command the orchestrator session prints and the human pastes.
Scripts are run through the `main` worktree: `../main/tools/wt/<script>`.

1. **Assign.** In the orchestrator session the human says what to build. Claude
   adds a row to `BOARD.md` with status `planned`, writes
   `handoffs/<dir>/00-brief.md` (goal, constraints, definition of done, links
   to specs), commits both on the `orchestrator` branch, and prints:

   ```bash
   ../main/tools/wt/new.sh feat/<name>
   cd ../feat-<name> && claude
   ```

2. **Build.** The feature session starts. The `SessionStart` hook injects the
   BOARD row and the latest file under `../orchestrator/handoffs/<dir>/` (the
   brief on a first run, the last handoff afterwards). The session works under
   `main`'s `CLAUDE.md` and, when present, `local/rules.md`.

3. **Hand off.** When the task is done, or when the context guard fires, the
   human (or Claude on its own) runs `/handoff`. The command writes
   `../orchestrator/handoffs/<dir>/<timestamp>.md`, moves the BOARD row to the
   right status, commits on the `orchestrator` branch, and prints the command
   to start the next session.

4. **Merge.** Back in the orchestrator session the human says the feature is
   done. Claude reads the last handoff, checks the BOARD row is `review`, and
   prints:

   ```bash
   ../main/tools/wt/merge.sh feat/<name>
   ```

   The script rebases the branch on `origin/main`, runs the gate inside the
   feature worktree, fast-forwards `main`, and pushes `main`. If any step fails
   it stops and prints what failed; nothing is force-pushed.

5. **Remove.** Claude prints:

   ```bash
   ../main/tools/wt/rm.sh feat/<name>
   ```

   The script refuses unless the branch is merged into `main` (override with
   `--force`), removes the worktree, deletes the local and remote branch, and
   removes the Claude project symlink for that worktree. Claude moves the BOARD
   row to `merged` and commits.

## 6. Scripts (`main/tools/wt/`)

All scripts: `bash`, `set -euo pipefail`, locate the workspace root by walking
up until a directory containing `.bare` is found, refuse to run if
preconditions fail, print each step with a `==>` prefix, and never delete
anything that is not reproducible without asking for `--force`.

| Script | Arguments | Does |
|---|---|---|
| `lib.sh` | sourced | root detection, `die`, `step`, `key_of` (path to Claude project key), `dir_of` (branch to directory) |
| `new.sh` | `<branch> [--from <branch>] [--no-install] [--push]` | create branch + worktree from `origin/main` (or `--from`), symlink `local/`, `pnpm install`, link Claude project dir, optionally push with upstream |
| `merge.sh` | `<branch> [--no-push]` | fetch, rebase feature on `origin/main`, run `pnpm run check` in the feature worktree, `git -C main merge --ff-only`, push `main` |
| `rm.sh` | `<branch> [--force]` | verify merged, `git worktree remove`, `git branch -d`, `git push origin --delete`, remove Claude symlink; `--force` for an abandoned branch |
| `status.sh` | none | `git worktree list` with ahead/behind `main` per branch, the BOARD table, and a warning if `orchestrator/.claude` differs from `main/.claude` |
| `claude-link.sh` | `<worktree-dir>` | create `~/.claude/projects/<worktree-key>` as a symlink to the `.bare` key directory (section 7); idempotent; backs up a real directory instead of deleting it |
| `init.sh` | `<repo-url> [<dir>]` | for a new project: bare clone, `.git` pointer, fetch refspec, `main` worktree, `orchestrator` worktree (creates the orphan branch if the remote has none), `local/` folder, `.bare/claude` symlink. This is the script `docs/infra/06-bootstrap-new-project.md` tells Claude to run. |

`new.sh` installs by default because an uninstalled worktree is the most common
cause of a confused first session. `--push` is opt-in so an experiment can
stay local.

`merge.sh` is a fast-forward merge on purpose: rebasing first keeps `main`
linear, which makes the open-source history readable and makes `git bisect`
usable. The gate is `pnpm run check`, defined in `package.json` as lint, then
typecheck, then test.

## 7. Claude Code data: memory, transcripts, handoffs

Facts, verified against the Claude Code docs on 2026-10-18:

- **Auto memory** lives at `~/.claude/projects/<key>/memory/`. For worktrees
  the key is derived from the git repository, so every worktree of this repo
  already shares one memory directory. With a bare layout the key is the
  `.bare` path: `-home-work-auth-gate--bare`. This is verified empirically as
  the first task of the implementation plan; if the key turns out different, the
  symlink target in `claude-link.sh` and `.bare/claude` follow the real value.
- **Transcripts** (`<session-id>.jsonl`) live under the key of the working
  directory, so each worktree gets its own transcript folder by default.
  `claude --resume` can still list sessions from every worktree of the repo
  (Ctrl+W in the picker) and resumes a session by id from any worktree.
- **Claude does not know its own context usage.** The statusline receives
  `context_window.used_percentage`; hooks on `UserPromptSubmit` and
  `SessionStart` can add text to the context.

Design:

1. **Memory:** use the default. Nothing to configure. `.bare/claude` is a
   symlink to the memory key directory so the human can find it.
2. **Transcripts:** `claude-link.sh` makes each worktree's project directory a
   symlink to the `.bare` key directory, so every transcript lands in one place
   and `.bare/claude` shows all of them. Whether Claude Code accepts a
   symlinked project directory is undocumented, so the plan includes a smoke
   test (run `claude -p` in a new worktree, confirm the transcript appears under
   the `.bare` key). If it fails, `new.sh` skips the link and the docs say to
   rely on cross-worktree `--resume`. Either way the project still works; only
   where the files sit changes.
3. **Knowledge continuity is the handoff, not the transcript.** A transcript is
   megabytes of tool output; nobody reads it. What a new session needs is: the
   assignment, what was done, the exact state of in-progress work, and the next
   command. That is a handoff file, and the `SessionStart` hook puts it in
   front of Claude before the first prompt.

### Handoff file format

```markdown
# Handoff: feat/pin — 2026-10-18 16:40

## Done
- ...

## In progress (exact state)
- file, branch state, failing test, whatever a stranger needs to continue

## Remaining
- ...

## Decisions and gotchas
- ...

## Next command
cd /home/work/auth-gate/feat-pin && claude
```

### BOARD.md format

```markdown
# Board

| Worktree | Branch | Task | Status | Last handoff | Notes |
|---|---|---|---|---|---|
| feat-pin | feat/pin | PIN factor | active | 2026-10-18-1640 | |

Status values: planned, active, review, merged, abandoned.
```

### Context guard

- `.claude/statusline.sh` reads the statusline JSON from stdin, prints
  `<branch> | ctx <used>% | $<cost>` for the status bar, and writes the used
  percentage to `/tmp/claude-ctx/<session_id>`.
- `.claude/hooks/context-guard.sh` runs on `UserPromptSubmit`. It reads that
  file. Below 70 % it prints nothing. From 70 % it prints one line telling
  Claude to finish the current step and run the handoff. From 85 % it prints a
  stronger line: stop, write the handoff now, print the new-session command.
  The hook output is added to the context, so Claude sees it on the next turn.
- `.claude/commands/handoff.md` is the `/handoff` slash command. It tells Claude
  to write the handoff file, update the BOARD row, commit on the `orchestrator`
  branch (`git -C ../orchestrator ...`), and end its reply with the command to
  start a new session.

The threshold is a number at the top of `context-guard.sh`, not scattered.

### SessionStart hook

Runs on `startup`, `resume`, `clear` and `compact`. Prints to stdout:

1. The BOARD row for the current branch (or "no assignment; ask the
   orchestrator" if none).
2. The newest file under `../orchestrator/handoffs/<dir>/`, capped at 120 lines.
3. One line pointing at `docs/infra/00-overview.md` for anything else.

In the orchestrator worktree the same hook prints the whole BOARD instead.

## 8. CLAUDE.md files

### `main/CLAUDE.md` (feature sessions, also the public one)

Short. Says: what the project is; code, comments, docs, commits in English;
the worktree layout in five lines; "your assignment and last handoff were
injected at session start; if they were not, read `../orchestrator/BOARD.md`";
run `/handoff` before stopping or when the context guard fires; commit only
when asked; never push `main` directly, `merge.sh` does that; `local/rules.md`
takes precedence when it exists.

### `orchestrator/CLAUDE.md`

Says: you coordinate, you do not write product code; `BOARD.md` is the single
source of truth and you keep it current; every action on a worktree is a
command block the human pastes, using `../main/tools/wt/`; one feature per
worktree; before printing `merge.sh` confirm the branch's last handoff says done
and the row is `review`; after `rm.sh` move the row to `merged`; when the
context guard fires, write your own handoff under `handoffs/orchestrator/` and
print the new-session command; commit on the `orchestrator` branch after every
BOARD change, since that branch is the state.

## 9. pnpm

- `package.json` at the `main` root declares `"packageManager": "pnpm@10.13.1"`
  and the scripts `lint`, `typecheck`, `test`, `check` (the three in order).
- `pnpm-workspace.yaml` lists `packages/*` and `apps/*`.
- `.npmrc` is committed with pnpm defaults and one comment line, so the place
  to put a future setting is obvious.
- Each worktree runs its own `pnpm install`. pnpm's content-addressable store
  (default `~/.local/share/pnpm/store`) is shared, so a second worktree costs
  hard links, not a second copy.
- On this machine `corepack` fails signature verification, so `pnpm` is the
  globally installed binary (`npm ls -g pnpm` shows 10.13.1). `docs/infra/05-pnpm.md`
  records the symptom and the fix (`npm i -g pnpm@10.13.1`, or
  `corepack disable`), so the next machine does not rediscover it.

## 10. `local/` and `.gitignore`

`.gitignore` is committed (the reference project did not commit its own, which
is why its bootstrap had to copy it by hand). Contents:

```gitignore
/local
node_modules/
.env
.env.*
!.env.example
.claude/settings.local.json
bruno/environments/*.yml
!bruno/environments/*.example.yml
```

`local/` holds `rules.md`, `working-philosophy.md`, the reference collections,
and anything with a secret in it. The infra docs describe what `local/` is for
without describing its contents.

## 11. Migrating this repository into the layout

The repo today is a normal checkout at `/home/work/auth-gate` with a `.git`
directory, no commits, a remote, and `local/`. The one-off migration:

1. Create the first commit on `main` in place: `.gitignore`, `README.md`,
   `LICENSE`, this spec. An unborn branch cannot be checked out into a
   worktree with git 2.39, so the first commit happens before the conversion.
2. `mv .git .bare` and set `core.bare = true` in `.bare/config`. The object
   database is unchanged; only its location and mode change.
3. `echo "gitdir: ./.bare" > .git`.
4. `git worktree add main main`. The worktree is populated from the commit, so
   the root-level copies of the committed files are deleted (not `local/`).
5. `git worktree add --detach orchestrator`, then inside it
   `git checkout --orphan orchestrator`, `git rm -rf .`, add `CLAUDE.md`,
   `BOARD.md`, `.claude/`, commit.
6. `ln -s ../local main/local`, `ln -s ../local orchestrator/local`.
7. Move the current session's memory: `~/.claude/projects/-home-work-auth-gate/memory/`
   becomes the content of `~/.claude/projects/-home-work-auth-gate--bare/memory/`
   (verify the key first, section 7). Run after this session ends.
8. `ln -s ~/.claude/projects/-home-work-auth-gate--bare .bare/claude`.
9. Push `main` and `orchestrator`.

The remaining round-1 work (scripts, hooks, docs, CI, empty pnpm workspace)
is normal feature work and goes through the lifecycle in section 5, on a branch
such as `feat/infra-tooling`, so the lifecycle is tested by building itself.

## 12. Verification (definition of done for round 1)

All of these must pass before round 1 is called done:

1. `git worktree list` shows `.bare`, `main`, `orchestrator`, and one feature
   worktree created by `new.sh`, each on the expected branch.
2. In the feature worktree: `ls -l local` is a symlink, `pnpm install` has
   run, `pnpm run check` passes on the empty workspace.
3. Starting `claude` in the feature worktree shows the BOARD row and the brief
   in the first context (visible in the first reply when asked "what is your
   assignment?").
4. The status bar shows the context percentage, and
   `/tmp/claude-ctx/<session_id>` contains a number.
5. Forcing the guard (temporarily setting the threshold to 0) makes the next
   prompt carry the warning line.
6. `/handoff` creates a file under `../orchestrator/handoffs/<dir>/`, updates
   the BOARD row, and the orchestrator branch has a new commit.
7. The transcript smoke test from section 7 has a recorded result in
   `docs/infra/04-claude-sessions.md`, whichever way it went.
8. `merge.sh` on the feature branch fast-forwards `main`, and `rm.sh` leaves no
   worktree, no local branch, no remote branch, and no dangling Claude symlink.
9. `init.sh` run in a scratch directory against the GitHub remote reproduces
   the layout (`.bare`, `main`, `orchestrator`, `local/`).
10. A second person (or a fresh Claude session given only `docs/infra/`) can
    explain the layout and the lifecycle without reading the scripts.

## 13. Open points carried to round 2

- The auth-gate module design (PIN + WebAuthn on Lambda + DynamoDB,
  backend-to-backend, Bruno collection as the integration handoff). Decided in
  conversation on 2026-10-18; the spec is written after round 1 is verified.
- Whether `.claude/` on the `orchestrator` branch should be a symlink to
  `../main/.claude` instead of a copy. A copy is chosen now because Claude Code
  treats symlinked settings pointing outside the project as external; revisit
  if drift becomes a problem.

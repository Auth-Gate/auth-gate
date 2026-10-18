# 3. Session continuity through handoff files, not transcripts

Date: 2026-10-18

Status: accepted

## Context

Claude Code sessions end: the context window fills, the human closes the terminal, or the
work moves to another day. A new session must know what the project is, what this
worktree was assigned, and where the previous session stopped.

What Claude Code offers:

- Auto memory under `~/.claude/projects/<key>/memory/`. For worktrees the key comes from
  the git repository, so all worktrees of one repo share it. It holds general facts, not
  the state of one task.
- Transcripts (`<session-id>.jsonl`) under the key of the working directory. They are
  megabytes of tool output. Nobody reads them, and resuming one brings back a full context.
- Claude does not know its own context usage. Only the statusline receives
  `context_window.used_percentage`. Hooks on `SessionStart` and `UserPromptSubmit` can add
  text to the context.

## Decision

- Memory: use the default. `.bare/claude` is a symlink to the shared key directory so the
  human can find it.
- Transcripts: `tools/wt/claude-link.sh` makes each worktree's project directory a
  symlink to the `.bare` key, so all transcripts sit in one place. This is a convenience.
- Continuity: a short handoff file per session end, written by `/handoff` to
  `../orchestrator/handoffs/<dir>/<yyyy-mm-dd-hhmm>.md` with the sections Done, In
  progress (exact state), Remaining, Decisions and gotchas, Next command.
- A `SessionStart` hook injects the BOARD row and the newest handoff (or the brief).
- The statusline records the context percentage; a `UserPromptSubmit` hook warns from
  70 % and orders a handoff from 85 %.

## Consequences

- A new session starts with a page of relevant state, not a transcript.
- The quality of continuity depends on the handoff being written. The context guard and
  `CLAUDE.md` both push for it.
- Handoffs are committed on `orchestrator`, so they have history and are backed up.
- If Claude Code stops honouring a symlinked project directory, only the transcript
  location changes; continuity does not depend on it.

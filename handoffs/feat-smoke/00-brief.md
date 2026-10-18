# Brief: feat/smoke

## Goal
Smoke-test the worktree flow end to end: add exactly one line to `README.md`.

## Constraints
- Change only `README.md`, one added line, nothing else.
- Follow the normal commit hooks; do not bypass them.
- Write a handoff (via the `handoff` skill) when done and set the BOARD row to `review`.

## Definition of done
- `README.md` has one new line; `git diff main --stat` shows only `README.md | 1 +`.
- Commit on `feat/smoke` passes hooks.
- Handoff written with an empty `Remaining` section.

## References
- `docs/infra/00-overview.md`
- `tools/wt/` scripts

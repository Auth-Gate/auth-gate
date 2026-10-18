# Brief: feat/auth-gate

## Goal
Implement v1 of the auth-gate module by executing
`handoffs/feat-auth-gate/plan.md` (15 tasks) with subagent-driven-development.
Task 0 lands `spec.md` and `plan.md` from this branch into `docs/superpowers/` on
the feature branch; from then on the copies under `docs/superpowers/` are the
source of truth and their checkboxes track progress.

## Constraints
- Follow the plan task by task; do not reorder or merge tasks.
- Tests only where the plan puts them (spec §11). No extra tests, no mocks of the
  AWS SDK, no coverage targets.
- Steps marked HUMAN (SSM writes, `sam deploy`, smoke against stg, pushes) are
  printed for the human, never run by Claude.
- Code, comments, docs, commits in English; chat in Vietnamese.
- `pnpm run check` must be green at every commit. Commit after every task.
- Do not copy `local/_reference-services/*/environments/stg.yml` (plaintext keys).
- Write a handoff (`/handoff`) at the end of every session and when the context
  guard fires; set the BOARD row to `active` on the first commit and `review` when
  Task 14 is done.

## Definition of done
- Spec §14: `pnpm run check` green locally and in CI; three Function URLs on stg;
  `04-smoke.sh` passes; Bruno collection clean against stg with `01-the-boundary/`
  failing as asserted; README + collection docs explain credential model, relay
  flow, token verification; all seven memos exist.
- Every checkbox in `docs/superpowers/plans/2026-10-18-auth-gate-module.md` ticked.
- Handoff with `Remaining: none`.

## References
- `handoffs/feat-auth-gate/spec.md` (orchestrator branch) → `docs/superpowers/specs/2026-10-18-auth-gate-module-design.md`
- `handoffs/feat-auth-gate/plan.md` (orchestrator branch) → `docs/superpowers/plans/2026-10-18-auth-gate-module.md`
- `local/rules.md`, `local/working-philosophy.md`
- `local/_reference-services/*/opencollection.yml` (sibling conventions, docs only)
- `docs/infra/02-worktree-lifecycle.md`

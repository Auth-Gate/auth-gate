# Contributing

## Commits

Use [Conventional Commits](https://www.conventionalcommits.org/):
`<type>(<optional scope>): <summary>`, for example `feat(wt): add --push to new.sh` or
`docs(infra): explain the context guard`. Common types: `feat`, `fix`, `docs`, `chore`,
`ci`, `test`, `refactor`. Write the summary in the imperative, in English.

## Branches

One feature per branch, named `feat/<name>` (or `fix/<name>`, `docs/<name>`). Branch from
`main`. `main` only moves by fast-forward after a rebase, so keep your branch rebased and
do not merge `main` into it.

If you use the worktree layout (`docs/infra/`), create the branch with:

```bash
../main/tools/wt/new.sh feat/<name>
```

## The gate

`pnpm run check` must pass before a branch is merged. It runs, in order:

- `pnpm run lint`: shellcheck over `tools/wt/` and `.claude/` scripts.
- `make check`: `go vet`, `golangci-lint run`, `go test`.
- `pnpm run test`: the hook test and the worktree end-to-end test.

CI runs the same command on every push to `main` and on every pull request.

## Running one test alone

```bash
bash tools/wt/test/e2e.sh     # worktree scripts, end to end, in a temp dir
bash tools/wt/test/hooks.sh   # statusline and hooks
```

Both are hermetic: they work in a temporary directory and do not touch your workspace,
your git config or `~/.claude`.

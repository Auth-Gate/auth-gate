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

## Go toolchain

`go.mod` says `go 1.26.0`. Install golangci-lint v2 and make sure `$(go env GOPATH)/bin`
is on your `PATH`, or `make check` cannot find it:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
make check    # go vet, golangci-lint run, go test ./...
```

## DynamoDB Local

The store tests need DynamoDB Local. Locally they skip unless `DYNAMO_ENDPOINT` is set (CI sets it), so a green `make check` alone
does not exercise them; run them before changing anything in `internal/platform/dynamo`
or the challenge and lockout code:

```bash
make test-store   # docker compose up -d dynamodb, then go test with DYNAMO_ENDPOINT=http://localhost:8000
make dev          # the same container, plus all three roles on :8080
```

CI runs the same container as a service.

## The gate

`pnpm run check` must pass before a branch is merged. It runs, in order:

- `pnpm run lint`: shellcheck over `scripts/setup-and-deploy/*.sh`, `tools/wt/*.sh`,
  `tools/wt/test/*.sh` and the `.claude/` scripts.
- `pnpm run test`: the shell tests, `tools/wt/test/hooks.sh` (statusline and hooks) and
  `tools/wt/test/e2e.sh` (worktree scripts, end to end).
- `make check`: `go vet`, `golangci-lint run`, `go test`.

CI runs the same command on every push to `main` and on every pull request.

## Running one test alone

```bash
bash tools/wt/test/e2e.sh     # worktree scripts, end to end, in a temp dir
bash tools/wt/test/hooks.sh   # statusline and hooks
```

Both are hermetic: they work in a temporary directory and do not touch your workspace,
your git config or `~/.claude`.

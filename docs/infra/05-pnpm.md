# pnpm

## Why

Every worktree is a full checkout and needs its own `node_modules`. With npm, five
worktrees mean five full copies of every dependency. pnpm keeps one content-addressable
store per machine (default `~/.local/share/pnpm/store`) and fills each `node_modules` with
hard links into it. A second worktree costs links, not a second copy, and installs are
fast. Each worktree still runs its own `pnpm install`, so worktrees on different branches
can have different dependencies.

## How

### Version pinning

`package.json` at the repository root:

```json
{
  "packageManager": "pnpm@10.13.1",
  "engines": { "node": ">=22" }
}
```

`packageManager` documents the version and is what CI's `pnpm/action-setup` reads.
`pnpm-workspace.yaml` lists the workspace members:

```yaml
packages:
  - packages/*
  - apps/*
```

`.npmrc` is committed with only a comment, so the place for a future setting is obvious.

### The corepack problem

On this machine, corepack's pnpm shim failed signature verification with:

```text
Cannot find matching keyid
```

The fix removes only corepack's pnpm shim and installs pnpm as a normal global package:

```bash
corepack disable pnpm
npm install -g pnpm@10.13.1
```

Check:

```bash
which pnpm && pnpm --version && pnpm store path
```

Expected: a path ending in `/bin/pnpm`, `10.13.1`, and a store path under
`~/.local/share/pnpm/store/`.

Do not run `corepack disable` without an argument. That removes the yarn shims too, and
other projects on the machine may rely on corepack for yarn.

### The gate

`pnpm run check` is the gate. `merge.sh` runs it in the feature worktree before
fast-forwarding `main`, and CI runs it on every push to `main` and every pull request.

```json
"scripts": {
  "lint": "shellcheck -x tools/wt/*.sh tools/wt/test/*.sh .claude/statusline.sh .claude/hooks/*.sh",
  "typecheck": "pnpm -r --if-present run typecheck",
  "test": "bash tools/wt/test/hooks.sh && bash tools/wt/test/e2e.sh && pnpm -r --if-present run test",
  "check": "pnpm run lint && pnpm run typecheck && pnpm run test"
}
```

- `lint` runs shellcheck from the `shellcheck` dev dependency (an npm package that ships
  the binary), so no system install is needed.
- `typecheck` and the package part of `test` use `pnpm -r --if-present`: each workspace
  package that defines the script runs it; packages that do not are skipped. With no
  packages yet, they succeed trivially.
- `test` runs the two hermetic shell tests first: `hooks.sh` (statusline and hooks) and
  `e2e.sh` (every `tools/wt` script against a throwaway remote in a temp folder).

The order is cheap to expensive, and `&&` stops at the first failure.

CI (`.github/workflows/ci.yml`) is checkout, `pnpm/action-setup@v4`, `setup-node@v4`
with Node 22 and the pnpm cache, `pnpm install --frozen-lockfile`, `pnpm run check`.

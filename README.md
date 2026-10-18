# auth-gate

auth-gate will be a two-factor verification service: a PIN and WebAuthn passkeys,
running on AWS Lambda, that other systems integrate with backend to backend through a
per-client key. This is round 1: the workspace, its tooling and an empty pnpm workspace.
The module itself is designed and built in round 2.

## Repository layout

```text
main/
├── .claude/                  # statusline, hooks, /handoff command (Claude Code)
├── docs/
│   ├── infra/                # how the workspace works, and how to rebuild it
│   ├── adr/                  # architecture decision records
│   └── superpowers/          # specs and plans
├── tools/wt/                 # worktree scripts
├── packages/                 # pnpm workspace packages (empty in round 1)
├── apps/                     # deployable apps (empty in round 1)
├── .github/workflows/ci.yml  # runs the gate
├── .gitignore                # /local, node_modules/, .env files
├── .npmrc                    # pnpm settings (defaults)
├── package.json              # pnpm@10.13.1; scripts lint, typecheck, test, check
├── pnpm-workspace.yaml
├── CLAUDE.md
├── CONTRIBUTING.md
└── LICENSE
```

## Developing

You need Node.js 22 or newer, pnpm 10.13.1, `jq` and `git`.

If corepack's pnpm shim fails with "Cannot find matching keyid", install pnpm directly
(see `docs/infra/05-pnpm.md`):

```bash
corepack disable pnpm
npm install -g pnpm@10.13.1
```

Then:

```bash
pnpm install
pnpm run check   # lint, then typecheck, then test
```

## Worktree workflow

The repository is used as a bare clone with one sibling folder per branch:

1. `.bare/` is the repository; `main/`, `orchestrator/` and `feat-<name>/` are worktrees.
2. The `orchestrator` branch holds `BOARD.md` (who does what) and handoff files.
3. `tools/wt/new.sh feat/<name>` creates `feat-<name>/`, installs, and links Claude data.
4. Each feature runs its own Claude Code session; `/handoff` records where it stopped.
5. `tools/wt/merge.sh feat/<name>` rebases, runs the gate, and fast-forwards `main`.
6. `tools/wt/rm.sh feat/<name>` removes the worktree and its branches.

A plain `git clone` also works for contributing. The full design is in
[`docs/infra/`](docs/infra/00-overview.md).

## License

MIT. See [LICENSE](LICENSE).

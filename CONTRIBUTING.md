# Contributing

Thanks for helping improve memtree! By participating you agree to the
[Code of Conduct](CODE_OF_CONDUCT.md), and contributions are licensed under
the [MIT License](LICENSE).

## Workflow

1. For anything non-trivial, open or comment on an issue first so we can
   agree on the approach.
2. Fork, create a branch (`feat/...`, `fix/...`, `docs/...`), and keep PRs
   focused.
3. Run `make test` and `make lint` before pushing. CI must be green and a
   maintainer review is required before merging into `main`; PRs are
   squash-merged.
4. Sign off every commit (`git commit -s`). This adds a `Signed-off-by:` line
   certifying, under the [Developer Certificate of Origin](https://developercert.org),
   that you have the right to submit the change. The DCO check fails without
   it; fix existing commits with `git rebase --signoff main`.
5. Use clear commit/PR titles, e.g. `fix: recall misses lattice-only matches`.
   Labels on the PR decide where it appears in the release notes; area labels
   are added automatically from the files you change.

## AI-assisted contributions

You're welcome to use AI tools (memtree is built for them), but you are the
author of your pull request:

- **Read and understand every line you submit**, and be ready to explain it
  in review.
- **Run it before opening the PR:** `make test` and `make lint` must pass
  locally. PRs that are clearly unreviewed output (placeholder text,
  truncated files, deleted tests, code that doesn't compile) will be closed.
- **Keep the change to the issue.** Don't let a tool rewrite or reformat files
  it didn't need to touch, and never remove tests to make something pass.
- **Write the PR description yourself,** and make sure it matches the diff.
- Signing off a commit (`git commit -s`) certifies you have the right to
  submit it, whoever or whatever helped write it.

## Finding something to work on

Issues move through these labels:

| Label | Meaning |
|---|---|
| `triage` | New; a maintainer hasn't looked at it yet. Issue forms add it automatically. |
| `needs repro` | Triaged, but we need steps or details to reproduce it before anyone can fix it. Without new information it closes after five weeks. |
| `ready` | Triaged and specified: [pick one up](https://github.com/aitakemyjob/memtree/issues?q=is%3Aopen+label%3Aready). |

`good first issue` marks small, well-scoped `ready` issues. `area: …` labels
say which part of memtree is involved, and a milestone means we'd like it in
that release.

To take a `ready` issue, comment that you're working on it so nobody
duplicates the effort. Open questions in an issue can be settled in the
issue thread before or during the PR.

## Setup

```sh
make tools deps   # Wails CLI, golangci-lint, govulncheck and frontend packages
make test         # Go with -race, frontend typecheck + tests
make lint         # golangci-lint and ESLint, as CI runs them
make vuln         # govulncheck and npm audit
make dev          # desktop app with hot reload
```

Without WebKitGTK headers on Linux you can still develop everything: build the
headless binary (`make headless`), run `memtree serve`, and run the Vite dev
server (`cd frontend && npm run dev`); it proxies `/api` and `/mcp` to
`127.0.0.1:47475`.

`go test -race` needs cgo. If your `PATH` has another toolchain's `as` ahead
of the system one (ESP-IDF, for instance) the race build fails with
`unrecognized option '--64'`; put `/usr/bin` first.

## Conventions

- **Go:** `make lint` clean (golangci-lint, configured in `.golangci.yml`;
  it includes gofmt and vet). Business logic belongs in
  `internal/core` (rules) and `internal/store` (persistence); `api`,
  `mcpserver`, `server` and `hooks` stay thin adapters. Prefer table-driven
  tests. Stores take an injected clock: never call `time.Now()` below `core`.
- **No model calls.** memtree runs no LLM. Summarising and classifying belong
  to the agent over MCP. Features that need judgement should be exposed as a
  tool the agent calls, not a model call inside memtree.
- **Memory is never silently destroyed.** Compression supersedes, agents
  archive, and only a human action in the UI deletes. Keep it that way.
- **Hooks fail open.** A hook must never block or break the agent: swallow
  errors, exit 0, print only what the harness should inject.
- **TypeScript:** strict mode, `npm run lint` (ESLint) clean with no
  warnings. Keep pure logic in `src/lib`
  with `*.test.ts` beside it; views consume the store and `api`.
- **Documentation:** every Go function and method has a doc comment
  (`docs_test.go` enforces this in CI), and every TypeScript function,
  component and API/store method has a JSDoc comment.
- **JSON contract:** Go structs in `store`/`core` are mirrored in
  `frontend/src/lib/types.ts`. Change both together, and return empty slices
  rather than `nil` for lists the UI iterates.
- **MCP tools:** every tool needs a description written for an LLM, a test in
  `internal/mcpserver/mcpserver_test.go` and an entry in `docs/MCP.md`
  (`TestToolsAreDocumented` enforces it).
- **Hooks for a new harness:** add a payload parser and output renderer in
  `internal/hooks/hooks.go`, install wiring in `install.go`, golden-payload
  tests built from the harness's documented format, and a row in
  `docs/HOOKS.md`. Say in the PR which documentation you checked: harness
  hook formats change.
- **Porting from cortex:** memtree's memory and lattice are a port. Where it
  deliberately differs (see `docs/ARCHITECTURE.md`, "Differences from the
  original"), keep the reason in a comment next to the code.

## Branding

The source logo is `logo.png` at the repo root. After changing it, run
`make icon` to regenerate `build/appicon.png` (desktop icons and Linux
packages), `frontend/public/logo.png`, `frontend/public/favicon.png` and
`docs/assets/logo.png` (README). The tool removes a baked-in checkerboard
background automatically.

## Releases

Run `make release VERSION=vX.Y.Z` from a clean `main`. It runs the tests,
tags and pushes; the Release workflow builds every platform with
`scripts/build.sh` / `scripts/build.ps1` and publishes the GitHub release
with checksums, an SPDX SBOM and signed build provenance. Release notes are
grouped by PR label (`.github/release.yml`); add user-facing changes to
`CHANGELOG.md` under *Unreleased* as you go. See the README's *Releasing*
section for signing secrets.

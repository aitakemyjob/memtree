# Contributing

Thanks for helping improve memtree! By participating you agree to the
[Code of Conduct](CODE_OF_CONDUCT.md), and contributions are licensed under
the [MIT License](LICENSE).

## Workflow

1. For anything non-trivial, open or comment on an issue first so we can
   agree on the approach.
2. Fork, create a branch (`feat/...`, `fix/...`, `docs/...`), and keep PRs
   focused.
3. Run `make test` before pushing. CI must be green and a maintainer review
   is required before merging into `main`; PRs are squash-merged.
4. Use clear commit/PR titles, e.g. `fix: recall misses lattice-only matches`.

## Setup

```sh
make tools deps   # Wails CLI and frontend packages
make test         # everything CI runs (Go with -race, frontend typecheck + tests)
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

- **Go:** `gofmt`, `go vet -tags nogui ./...` clean. Business logic belongs in
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
- **TypeScript:** strict mode, no unused locals. Keep pure logic in `src/lib`
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
`scripts/build.sh` / `scripts/build.ps1` and publishes the GitHub release.
See the README's *Releasing* section for signing secrets.

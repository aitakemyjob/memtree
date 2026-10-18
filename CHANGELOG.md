# Changelog

Notable changes to memtree. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org). Each
[GitHub release](https://github.com/aitakemyjob/memtree/releases) also lists
every merged pull request.

## [Unreleased]

### Added
- Persistent memory for coding agents: recall by wording and by lattice
  association, ACT-R activation decay, consolidation into semantic gists,
  project scoping, and a desktop/browser UI to inspect and edit it all.
- Hooks for Claude Code, Codex, Cursor and VS Code (session anchor, per-prompt
  recall, transcript capture) and an MCP server with 18 tools.
- Linting in CI (golangci-lint, ESLint), dependency vulnerability checks
  (govulncheck, npm audit) and OpenSSF Scorecard.
- Release builds ship an SPDX SBOM and signed build provenance; verify a
  download with `gh attestation verify <file> --repo aitakemyjob/memtree`.
- Release-note categories, automatic PR labels, DCO sign-off and a stale
  bot for issues waiting on reproduction details.

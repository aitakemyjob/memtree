# Hooks

`memtree install <harness>` wires hooks and the MCP server into a harness. It
merges into your existing config (never clobbering unrelated settings), saves a
one-time `.memtree.bak` backup, is idempotent, and `-dry-run` shows what it
would do. `-uninstall` removes only memtree's entries.

```sh
memtree install claude-code            # user scope
memtree install cursor -project .      # this repo only
memtree install vscode -project .      # VS Code is per workspace
memtree install codex -dry-run
```

MCP is registered as a **stdio bridge** (`memtree mcp`): it relays to the
running app so the UI sees every call live, and falls back to serving the same
database directly when the app is closed. Memory therefore always works.

| | Claude Code | Codex CLI | Cursor | VS Code (Copilot) |
|---|---|---|---|---|
| Hooks file | `~/.claude/settings.json` | `~/.codex/hooks.json` | `~/.cursor/hooks.json` | `.github/hooks/memtree.json` |
| MCP config | `claude mcp add` (user) / `.mcp.json` | `~/.codex/config.toml` | `~/.cursor/mcp.json` | `.vscode/mcp.json` |
| Session-start anchor | ✓ | ✓ | ✓ ¹ | ✓ |
| Per-prompt recall | ✓ | ✓ | ✗ ² | ? ³ |
| Capture user turns | ✓ | ✓ | ✓ | ✓ |
| Capture assistant turns | ✓ `Stop` | ✓ `Stop` | ✓ `afterAgentResponse` | ✗ ⁴ |
| Checkpoint / close | `Stop`, `PreCompact`, `SessionEnd` | same | `preCompact`, `sessionEnd` | `Stop`, `PreCompact` |

1. A Cursor forum report says `sessionStart` `additional_context` is not always
   injected; verify with the Activity view.
2. Cursor's `beforeSubmitPrompt` cannot inject context. Agents use the `recall`
   tool instead.
3. VS Code's two documentation pages disagree about `UserPromptSubmit` context.
4. VS Code's `Stop` payload has no last message; capturing assistant turns would
   mean parsing a transcript format that is not a stable API.

Codex asks you to trust new hooks: run `/hooks` in Codex after installing.

> **Status.** The hook formats above come from each vendor's documentation as
> of October 2026 and are covered by golden-payload tests built from those
> docs. They have **not** been exercised against live installs of every
> harness. Hook formats change: the Activity view shows every hook call
> (including errors), and `hook-errors.log` next to the config file records
> failures. Please report mismatches.

## What gets injected

- **Session start:** a short anchor: last session's notes in this project, a
  landmark, pinned notes, open loops, pending captures.
- **Each prompt:** up to 3 dated notes, only when something genuinely relates,
  phrased as facts that may be outdated. Capped at 8,000 characters (under
  Claude Code's 10,000 and Codex's ~2,500 tokens).

## Privacy

Hooks store the conversation turns they see in the local database. Turn
**Settings → Capture hooks** off to keep recall but stop recording. Nothing
leaves your machine.

# Security policy

memtree stores what you and your coding agents discuss, and injects it back
into agent context, so we take security reports seriously.

## Reporting a vulnerability

**Please don't open a public issue.** Report privately through
[GitHub's private vulnerability reporting](https://github.com/aitakemyjob/memtree/security/advisories/new).
Include what you found, how to reproduce it, and the impact you expect.

You can expect an acknowledgement within **3 business days** and a plan or
fix timeline within **10 business days**. We'll credit you in the advisory
unless you'd rather stay anonymous.

## Supported versions

Security fixes go into the latest release. Please upgrade before reporting.

## Scope

Especially interesting:

- Reaching the local HTTP/MCP server from a web page (CSRF, DNS rebinding,
  CORS) or from another machine
- Hook-payload or stored-memory content that escapes its context and acts as
  an instruction (prompt injection through recalled memory), or that breaks
  out of the JSON a harness parses
- The installer (`memtree install`) overwriting or corrupting harness
  configuration, or writing outside the harness's config locations
- An agent deleting memory when agent writes are off, or permanently deleting
  memory at all (agents may only archive)
- Reading or writing another project's memory without asking for it
- Script injection through memory text rendered in the UI

Out of scope: issues that require an already-compromised machine, or an
agent the user deliberately connected doing what the user asked.

## Data handling

memtree runs entirely on your machine, makes no network calls, and runs no
model. Captured conversation turns are stored unencrypted in the local SQLite
database (`~/.local/share/memtree/memtree.db` on Linux). Anything you type to
an agent while hooks are enabled can end up there, including secrets, so turn
capture off in **Settings → Capture hooks** for sensitive work.

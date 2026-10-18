# Architecture

memtree is one Go binary. The desktop shell (Wails), the browser mode, the MCP
server and the harness hooks are thin adapters over a single application
service, so the UI, agents and hooks see exactly the same memory through the
same rules.

```
        agent harness (Claude Code · Codex · Cursor · VS Code · any MCP client)
              │ hooks: `memtree hook <harness>`            │ MCP (stdio bridge or HTTP)
              ▼                                            ▼
   hookcmd.go ── running? ──yes──► POST /api/invoke        internal/mcpserver (tools)
        │ no                         hooks.handle                │
        ▼                                 │                      │
   direct DB mode                         ▼                      │
        └──────────────► internal/hooks (Manager) ───────────────┤
                                                                 ▼
   frontend (React + TS) ◄── Wails bindings │ HTTP + SSE ──► internal/api (JSON method table)
                                                                 │
                                                          internal/core  (Service)
                       ┌───────────────┬────────────────┬────────┴────────┬───────────────┐
                  store.Memory    store.Lattice    store.Captures    events (bus)       config
                       └───────────────┴──── one SQLite file (WAL, FTS5) ───┘
```

## Packages

| Package | Responsibility |
|---|---|
| `internal/store` | Persistence and the ported algorithms: `Memory` (records, FTS5, scoring, supersede/archive/compress), `Lattice` (category DAG, spreading activation, link decay, untagged queue), `Captures` (hook transcripts → raw capture memories), ACT-R `activation`, consolidation rules. Takes an injected clock; knows nothing about HTTP, MCP or harnesses. |
| `internal/core` | The `Service`: recall (text + lattice merge), per-prompt surfacing, the session-start anchor, lifecycle operations with events, the activity feed, maintenance. Everything the adapters share. |
| `internal/api` | The UI's JSON method table (`Invoke(method, args)`). |
| `internal/mcpserver` | MCP tools over `core`, rendered as compact text; every call is logged to the activity feed. |
| `internal/hooks` | `Manager`: parses each harness's hook payload, drives `core`, renders the harness-specific output; plus the installer that merges hook and MCP entries into harness config. |
| `internal/server` | Loopback HTTP: `/mcp` (streamable HTTP), `/healthz`, `/api/invoke`; in browser mode also the UI and `/api/events` (SSE). |
| `internal/events` | Non-blocking fan-out bus carrying memory/lattice/capture/activity events to UIs. |
| `internal/config` | Settings file and database location. |
| `main.go`, `gui.go`, `bridge.go`, `hookcmd.go` | CLI, Wails desktop shell (`!nogui`), stdio↔HTTP MCP bridge, and the hook entry point. |

## The model

### Memories

A memory is a short, self-contained note: summary, keywords, decisions, open
loops, **importance** (1 routine, 1.5 decision, 2.5 lesson), **tier**
(episodic = a specific event, semantic = distilled gist), **kind**, provenance
(`HUMAN_CONFIRMED` > `SOURCE_GROUNDED` > `MODEL_INFERRED`), a **project** (repo
root; empty = global, visible everywhere) and a **harness**.

Nothing is silently destroyed: compression *supersedes*, agents *archive*, and
only a human action in the UI deletes.

### Recall scoring

```
score = relevance × importance × activation × project boost × both bonus
```

- **relevance**: FTS5/BM25 (Porter stemming) relative to the best match in the
  result set (`bm25Normalized`). A lattice hit uses its link weight × node
  activation instead (`linkScore`). A memory both routes found takes the larger
  of the two and a **both bonus** of 1.15 (`bothBonus`), so the recorded
  factors always multiply out to the final score.
- **importance**: 1.0 / 1.5 / 2.5.
- **activation**: ACT-R base-level learning, `ln(Σ tᵢ^-d)` over the last 20
  access times (hours, min 1), squashed through a logistic into [0.15, 1].
  Episodic `d = 0.5`, semantic `d = 0.2` (gist should stay available). A new
  memory counts its creation as an access; being *relied on* (recall, or
  `reinforce`) adds one. Pinned memories have activation 1.
- **project boost**: 1.25 for the current project over a global memory.

Surfacing a memory in a result does **not** reinforce it; only reliance does.
The UI's "Why this score" view shows each factor.

### Retrieval traces

Every retrieval is recorded so a human can see *why* a memory came back and
*whether it was then used* (the Traces view, the inspector's retrieval
history, and the Timeline's Retrieved and Used colour modes). It is on by
default and can be switched off. Traces are kept for 90 days by default; the
period is configurable, and 0 keeps them forever.

- **What is recorded.** Recall, hook surfacing, session-start anchors and
  lattice probes each write one *event*:
  - Who asked: origin, tool or hook event, harness and session id. For hooks,
    the session id is the same one capture uses.
  - The query, plus the query tokens that fired each category.
  - Whether the hits were reinforced.
  - Each *hit*: its rank, route, the `seed → via` lattice path and its depth,
    and its score factors.

  An event is recorded even when nothing came back (a "quiet" event), which
  helps tune the surfacing threshold.
- **Use evidence.** Use is inferred by deterministic rules only; memtree runs
  no model. When an agent later acts on a retrieved memory within 30 minutes,
  the tracker credits the most closely related retrieval: the same session
  first, then the same project, then the newest. The rules that ship are:
  - **fetched**: the agent called `memory_get` on the memory.
  - **acted**: the agent called `memory_supersede`, `memory_update`,
    `memory_pin` or `memory_compress` on it.
  - **cited**: the memory's id appears in the agent's reply, as captured by the
    Stop hook.

  "Reinforced" (the hits were touched as relied on) is kept separate from
  "used", which requires evidence.
- **Seams.** `internal/trace` holds the model and the interfaces, and imports
  nothing else from memtree. The pieces:
  - `Recorder` writes traces and `Reader` queries them. `UseWriter` and
    `Pruner` complete the storage interfaces.
  - Each evidence rule is a `Detector`. `UseTracker` matches observations to
    recent retrievals and runs every registered detector.
  - `Retention` applies the configured period.
  - `store.Traces` is the SQLite implementation.
  - Adapters (MCP, hooks, the API, the CLI) only attach a `trace.Origin` to the
    context and report `Observation`s. Recording itself happens once, in
    `core`, so the hook fallback that opens the database directly records too.
- **Extending.** A new evidence rule is a new `Detector` added to the tracker's
  list. A new MCP tool counts as evidence when its arguments implement
  `memoryIDs()`. A new Timeline colour is an entry in `HEAT_MODES`
  (`frontend/src/lib/treemap.ts`).

### The lattice

A DAG of slash-path categories (`project/architecture/db`). Seeded axes:
`self`, `project`, `knowledge`, `person`, `action`, `domain`, `world`, `time`.
Agents grow children beneath them. A memory links to 1–4 categories with a
weight that decays by half-life (30 days) and is reinforced by use; faded links
are dropped, nodes never are.

**Firing:** a prompt's content words are matched against node labels (exact,
or ≥4-character containment). Those nodes are the seeds. **Spreading
activation** then runs a bounded BFS (depth 2, ×0.5 per hop) to parents and
children; each node's activation is scaled by its own time decay; linked
memories score `link weight × node activation`. No model, ~10 ms.

### Classification without a model

memtree runs no LLM. The agent classifies by passing `lattice_paths` to
`memory_save`. A memory saved without paths gets a provisional placement by
keyword-matching existing nodes (weight 0.35, below the surfacing threshold,
marked `auto`) and joins the **unclassified queue**, which the UI and the
`lattice_untagged` / `lattice_tag` tools work through.

### Hooks and capture

Hooks call `memtree hook <harness>`. It uses the running service if there is
one (so the UI sees traffic live) and otherwise opens the database directly
(WAL + busy timeout let both coexist). It **fails open**: it always exits 0,
logs problems to `hook-errors.log`, and prints only what the harness should
inject.

| Event | What memtree does |
|---|---|
| Session start | Injects the **anchor**: last session notes in this project, one older landmark (importance 2.5), pinned notes, open loops, and how many captured sessions still lack a summary. Never counts the current session. |
| Prompt submit | Records the user turn; injects up to 3 relevant notes (text match + lattice hits above the threshold), phrased as dated, possibly-stale facts. Prints nothing when nothing relates. |
| Stop / response | Records the assistant turn and **checkpoints** the session's raw capture, because session-end is not guaranteed. |
| Pre-compact, session end | Checkpoint / close. |

A closed session becomes a `capture` memory (a trimmed digest of the first
prompts and last answer, so it is findable by keyword). The agent later
summarises it (`captures_pending` → `capture_transcript` → `memory_save` with
`kind: session` and the `session_id`); the raw capture is then superseded.

### Consolidation

Old, rarely used, related memories are candidates for distilling: an episodic
record at least 72 h old (semantic: 720 h) whose activation is at most 0.5,
clustered greedily by project and keyword overlap (Jaccard ≥ 0.2). memtree
proposes clusters (`consolidation_candidates`); the agent (or you, in the UI)
writes the gist, and `memory_compress` stores it as a semantic memory and
supersedes the sources. The gist cannot inflate what it came from: importance
is the sources' maximum, provenance their weakest. Pinned memories are never
compressed.

## Design principles

- **One service, many adapters.** Rules live in `core`/`store`; the UI API, MCP
  tools and hooks only translate.
- **Local only.** Binds 127.0.0.1, rejects non-loopback `Host` (DNS rebinding),
  requires a custom header on the API (CSRF), makes no network calls.
- **No model inside.** All judgement is the agent's, through tools.
- **Humans can see and override everything.** The activity feed shows every
  agent and hook call; agent writes can be switched off; deletion is
  human-only.
- **Fail open.** Memory is an aid; an outage must never block the agent.
- **Headless-testable.** `-tags nogui` builds everything except the webview.

## Differences from the original (cortex)

memtree's memory and lattice are a port of the Cortex agent's. Deliberate
differences:

1. **Relevance is relative to the best match**, not an absolute BM25 squash.
   SQLite's `bm25()` collapses toward 0 in a small store (a term in half the
   documents has IDF ≈ 0), which made every text hit look irrelevant next to
   lattice hits. Cortex's own comments note absolute BM25 thresholds are
   unusable.
2. **Creation seeds the access log.** Cortex started records with an empty
   log, leaving every unused memory at the activation floor regardless of age.
3. **`DecayLinks` stamps `last_accessed`.** Cortex aged from the original
   access time each pass, so repeated passes compounded the same interval.
4. **Surfacing threshold 0.25, not 0.6.** A node's activation is at most
   ≈0.575 (a just-touched node: `ln(1) = 0`, logistic 0.5, floor-adjusted), so
   a 0.6 gate on `weight × activation` cannot be met. This is derived from
   reading cortex's code; we did not check what cortex's runtime config sets.
5. **Project + harness scoping replaces person scoping.** Global memories
   (project empty) are visible everywhere, as legacy records were.
6. **Agent-driven summarising, classifying and distilling** instead of
   LLM passes inside the store.
7. Out of scope: personality, reflection, inner-life, domains/RAG, goals,
   desires and the other cortex lattice item types.

# MCP tools

memtree serves MCP at `http://127.0.0.1:47475/mcp` (streamable HTTP) while the
app or `memtree serve` is running, and over stdio with `memtree mcp` (for
Claude Desktop and other stdio-only clients). All tools take an optional
`project`: pass your working directory and memtree maps it to the repo root.
Global memories are visible in every project.

memtree runs **no model**. Summarising and classifying is the agent's job; memtree
stores, ranks, forgets gradually and shows it all to the human in the UI.

## Recall

| Tool | Purpose |
|---|---|
| `recall` | Search memory by wording **and** lattice category. Returns ids, ages, scores and why each matched. Recalled memories are reinforced. Each call is recorded as a retrieval trace; a later `memory_get`, `memory_update`, `memory_supersede`, `memory_pin` or `memory_compress` on a returned id is recorded as evidence that it was used (see [Retrieval traces](ARCHITECTURE.md#retrieval-traces)). |
| `memory_get` | Read one memory in full, with lattice paths and lineage. |
| `memory_list` | List recent memories (pinned first) without searching. |
| `lattice_browse` | Show the category lattice as a tree with counts; optionally list memories filed under a path. |
| `lattice_activate` | Dry-run the lattice on some text: which categories light up and which memories surface. |

## Saving and classifying

| Tool | Purpose |
|---|---|
| `memory_save` | Save a decision, lesson, preference or session summary, with lattice paths, kind and importance. |
| `memory_update` | Edit a memory's text, keywords, importance or paths. |
| `memory_pin` | Pin a memory so it never fades or compresses. |
| `memory_supersede` | Mark a memory as replaced by a newer one; it stops appearing in recall but is kept. |
| `memory_archive` | Hide a wrong memory from recall (reversible, never deleted). |
| `lattice_tag` | Set a memory's lattice paths (classification). |
| `lattice_untagged` | List memories still waiting for classification. |
| `lattice_merge` | Fold near-duplicate categories together. |
| `lattice_lint` | List near-duplicate categories, stray roots, single-use categories and memories without a confirmed placement. |

## Captured sessions

Hooks record conversation turns automatically. A closed session becomes a raw
capture memory until the agent summarises it.

| Tool | Purpose |
|---|---|
| `captures_pending` | List captured sessions without a real summary yet. |
| `capture_transcript` | Read a captured session's turns. |

## Consolidation

Old, rarely used, related memories can be distilled into one semantic memory.
memtree proposes clusters; the agent writes the gist.

| Tool | Purpose |
|---|---|
| `consolidation_candidates` | Find clusters of aged, cooled, related memories. |
| `memory_compress` | Save the gist and supersede the originals (never deleted). |

## Recommended agent workflow

1. At the start of non-trivial work, `recall` the topic.
2. Before saving, `lattice_browse` so you reuse existing categories.
3. `memory_save` decisions (with the reason), hard-won lessons and preferences,
   with `lattice_paths`, `kind` and `importance` (1 routine, 1.5 decision, 2.5 lesson).
4. When something changes, `memory_save` the new fact and `memory_supersede` the old one.
5. When `captures_pending` is non-empty, summarise the sessions with `memory_save`
   (`kind: "session"`, `session_id`).
6. Occasionally run `lattice_lint` and fix what it finds, then `consolidation_candidates` and `memory_compress`.

`memory_save` and `lattice_tag` reply with **lattice advice** when a path is
outside the standard axes or resembles an existing category, naming the one to
use instead. They still file where you asked; the advice is yours to act on.

Switch agent writes off under **Settings → Agent access** for a read-only setup.

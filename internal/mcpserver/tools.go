package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/memtree/internal/core"
	"github.com/aitakemyjob/memtree/internal/store"
)

type recallArgs struct {
	Query             string `json:"query" jsonschema:"what to remember: a topic, question or phrase"`
	Project           string `json:"project,omitempty" jsonschema:"your current working directory; memtree maps it to the repo root and scopes memory to it (global memories are always included)"`
	AllProjects       bool   `json:"all_projects,omitempty" jsonschema:"search every project, not just this one"`
	Limit             int    `json:"limit,omitempty" jsonschema:"max memories (default 8)"`
	Since             string `json:"since,omitempty" jsonschema:"only memories newer than this: RFC 3339, YYYY-MM-DD, or a relative age like 7d, 24h, 2w, 3mo"`
	Until             string `json:"until,omitempty" jsonschema:"only memories older than this (same formats as since)"`
	Tier              string `json:"tier,omitempty" jsonschema:"episodic (specific events) or semantic (distilled gist)"`
	Kind              string `json:"kind,omitempty" jsonschema:"note, decision, lesson, session or capture"`
	IncludeSuperseded bool   `json:"include_superseded,omitempty" jsonschema:"also return memories that were replaced by newer ones"`
	NoReinforce       bool   `json:"no_reinforce,omitempty" jsonschema:"browse without marking these memories as used (default: recalled memories are reinforced)"`
}

type idArgs struct {
	ID string `json:"id" jsonschema:"memory id, e.g. mem_ab12cd34"`
}

type listArgs struct {
	Project     string `json:"project,omitempty" jsonschema:"your current working directory; memtree maps it to the repo root and scopes memory to it (global memories are always included)"`
	AllProjects bool   `json:"all_projects,omitempty" jsonschema:"list every project"`
	Kind        string `json:"kind,omitempty" jsonschema:"filter by kind"`
	Tier        string `json:"tier,omitempty" jsonschema:"episodic or semantic"`
	PinnedOnly  bool   `json:"pinned_only,omitempty" jsonschema:"only pinned memories"`
	Since       string `json:"since,omitempty" jsonschema:"only newer than this (RFC 3339, YYYY-MM-DD, or 7d/24h/2w)"`
	Limit       int    `json:"limit,omitempty" jsonschema:"max memories (default 20)"`
}

type saveArgs struct {
	Summary      string   `json:"summary" jsonschema:"the memory, 1-4 sentences, self-contained: someone reading it cold must understand it"`
	Project      string   `json:"project,omitempty" jsonschema:"your current working directory; memtree maps it to the repo root and scopes memory to it (global memories are always included); omit for a global memory (e.g. a personal preference)"`
	Global       bool     `json:"global,omitempty" jsonschema:"store as a global memory visible in every project"`
	Kind         string   `json:"kind,omitempty" jsonschema:"note (default), decision, lesson, or session (a summary of a captured session; pass session_id)"`
	Importance   float64  `json:"importance,omitempty" jsonschema:"1 routine (default), 1.5 a decision, 2.5 a hard-won lesson; this multiplies its recall score"`
	LatticePaths []string `json:"lattice_paths,omitempty" jsonschema:"1-4 category paths, lowercase slash paths rooted at an axis (project, knowledge, self, person, action, domain, world, time), e.g. project/architecture/db; call lattice_browse first and reuse existing ones"`
	Keywords     []string `json:"keywords,omitempty" jsonschema:"search terms; derived from the summary if omitted"`
	Decisions    []string `json:"decisions,omitempty" jsonschema:"explicit decisions made, ideally with the reason"`
	OpenLoops    []string `json:"open_loops,omitempty" jsonschema:"unfinished work to pick up next time"`
	Provenance   string   `json:"provenance,omitempty" jsonschema:"HUMAN_CONFIRMED (the user said so), SOURCE_GROUNDED (read from code/docs), MODEL_INFERRED (your inference; default)"`
	SessionID    string   `json:"session_id,omitempty" jsonschema:"with kind=session: the captured session this summarises (see captures_pending); retires its raw capture"`
	Pin          bool     `json:"pin,omitempty" jsonschema:"pin so it never fades or compresses; use sparingly for standing rules"`
}

type updateArgs struct {
	ID           string   `json:"id"`
	Summary      *string  `json:"summary,omitempty" jsonschema:"replace the text"`
	Keywords     []string `json:"keywords,omitempty" jsonschema:"replace keywords"`
	Decisions    []string `json:"decisions,omitempty" jsonschema:"replace decisions"`
	OpenLoops    []string `json:"open_loops,omitempty" jsonschema:"replace open loops"`
	Importance   *float64 `json:"importance,omitempty" jsonschema:"1, 1.5 or 2.5"`
	Kind         *string  `json:"kind,omitempty" jsonschema:"note, decision, lesson, session"`
	LatticePaths []string `json:"lattice_paths,omitempty" jsonschema:"replace the memory's lattice paths"`
}

type pinArgs struct {
	ID  string `json:"id"`
	Off bool   `json:"off,omitempty" jsonschema:"unpin instead of pin"`
}

type supersedeArgs struct {
	ID string `json:"id" jsonschema:"the old, outdated memory"`
	By string `json:"by" jsonschema:"id of the newer memory that replaces it"`
}

type archiveArgs struct {
	ID   string `json:"id"`
	Undo bool   `json:"undo,omitempty" jsonschema:"restore an archived memory"`
}

type tagArgs struct {
	ID    string   `json:"id"`
	Paths []string `json:"paths" jsonschema:"1-4 category paths replacing the memory's current ones"`
}

type browseArgs struct {
	Path  string `json:"path,omitempty" jsonschema:"category to open (e.g. project/architecture); omit for the whole lattice"`
	Depth int    `json:"depth,omitempty" jsonschema:"levels to show (default 3)"`
	Items bool   `json:"items,omitempty" jsonschema:"also list the memories filed under path and below"`
}

type fireArgs struct {
	Text string `json:"text" jsonschema:"a prompt or phrase; shows which categories it lights up and which memories surface"`
}

type projectArgs struct {
	Project     string `json:"project,omitempty" jsonschema:"your current working directory; memtree maps it to the repo root and scopes memory to it (global memories are always included)"`
	AllProjects bool   `json:"all_projects,omitempty"`
}

type sessionArgs struct {
	SessionID string `json:"session_id"`
}

type compressArgs struct {
	IDs          []string `json:"ids" jsonschema:"the memories being distilled (from consolidation_candidates)"`
	Summary      string   `json:"summary" jsonschema:"the distilled gist covering all of them; keep every decision and lesson that still holds"`
	Keywords     []string `json:"keywords,omitempty"`
	Decisions    []string `json:"decisions,omitempty"`
	OpenLoops    []string `json:"open_loops,omitempty" jsonschema:"only loops still open"`
	LatticePaths []string `json:"lattice_paths,omitempty" jsonschema:"defaults to the union of the sources' paths"`
}

type mergeArgs struct {
	From string `json:"from" jsonschema:"category to fold away"`
	To   string `json:"to" jsonschema:"category that keeps the links"`
}

// memoryIDs names the memory a single-id tool call is about.
func (a idArgs) memoryIDs() []string { return []string{a.ID} }

// memoryIDs names the memory being updated.
func (a updateArgs) memoryIDs() []string { return []string{a.ID} }

// memoryIDs names the memory being pinned or unpinned.
func (a pinArgs) memoryIDs() []string { return []string{a.ID} }

// memoryIDs names the memory being superseded.
func (a supersedeArgs) memoryIDs() []string { return []string{a.ID} }

// memoryIDs names the memories being distilled.
func (a compressArgs) memoryIDs() []string { return a.IDs }

// resolve maps an agent's working directory to a project key.
func (s *Server) resolve(project string) string { return core.ResolveProject(project) }

// register registers every MCP tool.
func (s *Server) register() {
	c := s.core
	addTool(s, &mcp.Tool{
		Name: "recall",
		Description: "Search the user's persistent memory for earlier decisions, lessons, preferences and session summaries. " +
			"Matches by wording AND by lattice category (so related notes surface even without shared words). " +
			"Use it when the user refers to earlier work or conventions, before re-deciding something, and at the start of non-trivial tasks. " +
			"Results include id, age, score and why each matched; older notes may be stale, so verify against the code. " +
			"Recalled memories are reinforced (they stay sharp) unless no_reinforce is set.",
		Annotations: &mcp.ToolAnnotations{Title: "Recall memory", IdempotentHint: false, OpenWorldHint: boolp(false), DestructiveHint: boolp(false)},
	}, func(a recallArgs) (string, string) { return a.Query, a.Project },
		func(ctx context.Context, a recallArgs) (string, error) {
			now := c.DB.Now()
			since, err := parseWhen(a.Since, now)
			if err != nil {
				return "", err
			}
			until, err := parseWhen(a.Until, now)
			if err != nil {
				return "", err
			}
			res, err := c.Recall(ctx, core.RecallInput{
				Query: a.Query, Project: s.resolve(a.Project), AllProjects: a.AllProjects, Limit: a.Limit,
				After: since, Before: until, Tier: a.Tier, Kind: a.Kind, IncludeSuperseded: a.IncludeSuperseded,
				Reinforce: !a.NoReinforce,
			})
			if err != nil {
				return "", err
			}
			if len(res.Memories) == 0 {
				return "No matching memories." + categoriesLine(res.Categories), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d memories%s\n\n", len(res.Memories), categoriesLine(res.Categories))
			for _, m := range res.Memories {
				why := fmt.Sprintf("score %.2f via %s", m.Score.Final, m.Source)
				if m.Via != "" {
					why += " (" + m.Via + ")"
				}
				b.WriteString(recordBlock(m.Record, now, why))
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "memory_get", Description: "Read one memory in full by id, including its lattice paths and, if it was distilled from others, what it replaced.",
		Annotations: readOnly("Get memory")},
		func(a idArgs) (string, string) { return a.ID, "" },
		func(ctx context.Context, a idArgs) (string, error) {
			r, err := c.Memory.Get(ctx, a.ID)
			if err != nil {
				return "", err
			}
			out := recordBlock(r, c.DB.Now(), "")
			if lin, _ := c.Memory.Lineage(ctx, a.ID); len(lin) > 0 {
				out += fmt.Sprintf("  distilled from %d memories: ", len(lin))
				ids := make([]string, len(lin))
				for i, l := range lin {
					ids[i] = l.ID
				}
				out += strings.Join(ids, ", ") + "\n"
			}
			return strings.TrimRight(out, "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "memory_list", Description: "List memories newest first (pinned first) without searching: what was recently saved in this project.",
		Annotations: readOnly("List memories")},
		func(a listArgs) (string, string) { return a.Kind, a.Project },
		func(ctx context.Context, a listArgs) (string, error) {
			since, err := parseWhen(a.Since, c.DB.Now())
			if err != nil {
				return "", err
			}
			lim := a.Limit
			if lim <= 0 {
				lim = 20
			}
			recs, err := c.Memory.List(ctx, store.Query{Project: s.resolve(a.Project), AllProjects: a.AllProjects, Kind: a.Kind, Tier: a.Tier, PinnedOnly: a.PinnedOnly, After: since, Limit: lim})
			if err != nil {
				return "", err
			}
			if len(recs) == 0 {
				return "No memories.", nil
			}
			var b strings.Builder
			for _, r := range recs {
				b.WriteString(recordBlock(r, c.DB.Now(), ""))
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "memory_save",
		Description: "Save something worth remembering across sessions: a decision and its reason, a hard-won lesson, a user preference, a convention, or a summary of meaningful work. " +
			"Write the summary so it stands alone. Classify it yourself: give lattice_paths (reuse existing categories from lattice_browse), a kind and an importance. " +
			"Do not save trivia, secrets, or what the code already says. If it replaces an older memory, also call memory_supersede.",
		Annotations: writes("Save memory", false)},
		func(a saveArgs) (string, string) { return a.Summary, a.Project },
		func(ctx context.Context, a saveArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			project := s.resolve(a.Project)
			if a.Global {
				project = ""
			}
			if a.Kind == store.KindCapture {
				return "", errors.New("kind capture is reserved for raw hook captures; use note, decision, lesson or session")
			}
			advice := s.pathAdvice(ctx, a.LatticePaths) // before saving creates the nodes
			r, err := c.Save(ctx, store.MemoryRecord{
				Summary: a.Summary, Project: project, Harness: "mcp", Kind: a.Kind, Importance: a.Importance,
				LatticePaths: a.LatticePaths, Keywords: a.Keywords, Decisions: a.Decisions, OpenLoops: a.OpenLoops,
				Provenance: a.Provenance, SessionID: a.SessionID, Pinned: a.Pin,
			})
			if err != nil {
				return "", err
			}
			note := ""
			if len(a.LatticePaths) == 0 {
				note = "\nNo lattice_paths given: it is queued as unclassified. Add them with lattice_tag when you can."
			}
			return "Saved " + r.ID + " (" + importanceName(r.Importance) + ", " + scopeName(r.Project) + ")." + note + advice, nil
		})

	addTool(s, &mcp.Tool{Name: "memory_update", Description: "Edit a memory you saved: fix its text, keywords, importance or lattice paths. Prefer memory_supersede when the facts changed over time.",
		Annotations: writes("Update memory", true)},
		func(a updateArgs) (string, string) { return a.ID, "" },
		func(ctx context.Context, a updateArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			var paths []string
			if a.LatticePaths != nil {
				paths = a.LatticePaths
			}
			r, err := c.Update(ctx, a.ID, func(r *store.MemoryRecord) {
				if a.Summary != nil {
					r.Summary = *a.Summary
				}
				if a.Keywords != nil {
					r.Keywords = a.Keywords
				}
				if a.Decisions != nil {
					r.Decisions = a.Decisions
				}
				if a.OpenLoops != nil {
					r.OpenLoops = a.OpenLoops
				}
				if a.Importance != nil {
					r.Importance = *a.Importance
				}
				if a.Kind != nil {
					r.Kind = *a.Kind
				}
			}, paths)
			if err != nil {
				return "", err
			}
			return "Updated " + r.ID + ".", nil
		})

	addTool(s, &mcp.Tool{Name: "memory_pin", Description: "Pin a memory so it never fades from recall and is never compressed; for standing rules and conventions.",
		Annotations: writes("Pin memory", true)},
		func(a pinArgs) (string, string) { return a.ID, "" },
		func(ctx context.Context, a pinArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			if err := c.SetPinned(ctx, a.ID, !a.Off); err != nil {
				return "", err
			}
			if a.Off {
				return "Unpinned " + a.ID + ".", nil
			}
			return "Pinned " + a.ID + ".", nil
		})

	addTool(s, &mcp.Tool{Name: "memory_supersede", Description: "Mark an outdated memory as replaced by a newer one. The old one is kept but stops appearing in recall. Use when facts or decisions changed.",
		Annotations: writes("Supersede memory", true)},
		func(a supersedeArgs) (string, string) { return a.ID + " → " + a.By, "" },
		func(ctx context.Context, a supersedeArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			if err := c.Supersede(ctx, a.ID, a.By); err != nil {
				return "", err
			}
			return a.ID + " is now superseded by " + a.By + ".", nil
		})

	addTool(s, &mcp.Tool{Name: "memory_archive", Description: "Hide a wrong or irrelevant memory from recall. It is not deleted; the user can restore it in the UI (or pass undo).",
		Annotations: writes("Archive memory", true)},
		func(a archiveArgs) (string, string) { return a.ID, "" },
		func(ctx context.Context, a archiveArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			if a.Undo {
				if err := c.Restore(ctx, a.ID); err != nil {
					return "", err
				}
				return "Restored " + a.ID + ".", nil
			}
			if err := c.Archive(ctx, a.ID); err != nil {
				return "", err
			}
			return "Archived " + a.ID + ".", nil
		})

	addTool(s, &mcp.Tool{Name: "lattice_browse",
		Description: "Show how memory is organised: the category lattice as a tree with memory counts. Call it before saving so you reuse existing categories instead of coining near-duplicates. " +
			"With items=true, also lists the memories filed under the given path and its subcategories.",
		Annotations: readOnly("Browse lattice")},
		func(a browseArgs) (string, string) { return a.Path, "" },
		func(ctx context.Context, a browseArgs) (string, error) {
			nodes, err := c.Lattice.Nodes(ctx)
			if err != nil {
				return "", err
			}
			path := store.NormalizePath(a.Path)
			depth := a.Depth
			if depth <= 0 {
				depth = 3
			}
			// Show only the requested subtree.
			var sub []store.CategoryNode
			for _, n := range nodes {
				if path == "" || n.ID == path || strings.HasPrefix(n.ID, path+"/") {
					sub = append(sub, n)
				}
			}
			if len(sub) == 0 {
				return "No such category: " + path + ". Roots: " + strings.Join(store.AxisRoots(), ", "), nil
			}
			out := renderTree(sub, path, depth)
			if a.Items && path != "" {
				var lines []string
				for _, n := range sub {
					if n.LinkCount == 0 {
						continue
					}
					links, err := c.Lattice.LinksFor(ctx, n.ID)
					if err != nil {
						return "", err
					}
					for _, l := range links {
						lines = append(lines, fmt.Sprintf("- %s  %s  weight %.2f  %s", l.ItemID, n.ID, l.Weight, oneLine(l.Summary, 100)))
					}
					if len(lines) >= 50 {
						break
					}
				}
				if len(lines) > 0 {
					out += "\n\nFiled under " + path + ":\n" + strings.Join(lines, "\n")
				}
			}
			return out, nil
		})

	addTool(s, &mcp.Tool{Name: "lattice_activate", Description: "Dry-run the lattice on some text: which categories it lights up and which memories would surface, with scores. Use to check how a topic is classified.",
		Annotations: readOnly("Activate lattice")},
		func(a fireArgs) (string, string) { return a.Text, "" },
		func(ctx context.Context, a fireArgs) (string, error) {
			seeds, hits, err := c.Probe(ctx, a.Text, 12)
			if err != nil {
				return "", err
			}
			if len(seeds) == 0 {
				return "Nothing in the lattice matches that text.", nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Lights up: %s\n", strings.Join(seeds, ", "))
			for _, h := range hits {
				fmt.Fprintf(&b, "- %s  %.2f  via %s  %s\n", h.ItemID, h.Score, h.ViaCategory, oneLine(h.Summary, 100))
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "lattice_tag", Description: "Classify a memory: set its lattice paths (replacing the current ones). Use for memories that arrived unclassified. Reuse existing categories (lattice_browse).",
		Annotations: writes("Tag memory", true)},
		func(a tagArgs) (string, string) { return a.ID, "" },
		func(ctx context.Context, a tagArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			advice := s.pathAdvice(ctx, a.Paths) // before tagging creates the nodes
			if err := c.TagMemory(ctx, a.ID, a.Paths, store.SourceAgent); err != nil {
				return "", err
			}
			return "Tagged " + a.ID + " under " + strings.Join(a.Paths, ", ") + "." + advice, nil
		})

	addTool(s, &mcp.Tool{Name: "lattice_untagged", Description: "List memories still waiting for classification (saved without lattice_paths, or captured by hooks). Classify them with lattice_tag.",
		Annotations: readOnly("Unclassified memories")},
		func(a struct{}) (string, string) { return "", "" },
		func(ctx context.Context, _ struct{}) (string, error) {
			items, err := c.Lattice.Untagged(ctx, 30)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "Nothing is waiting for classification.", nil
			}
			var b strings.Builder
			for _, it := range items {
				fmt.Fprintf(&b, "- %s  %s\n", it.ItemID, oneLine(it.Text, 140))
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "lattice_lint",
		Description: "Check the lattice's hygiene: near-duplicate sibling categories (merge them with lattice_merge), categories outside the standard axes, single-use categories, and memories with no confirmed placement (classify them with lattice_tag). Run it after filing a batch of memories or when asked to tidy memory.",
		Annotations: readOnly("Lint lattice")},
		func(a struct{}) (string, string) { return "", "" },
		func(ctx context.Context, _ struct{}) (string, error) {
			rep, err := c.Memory.Lint(ctx)
			if err != nil {
				return "", err
			}
			if rep.Total == 0 {
				return "The lattice is tidy: no near-duplicates, stray roots, single-use categories or weakly filed memories.", nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d finding(s).\n", rep.Total)
			if len(rep.NearDuplicates) > 0 {
				b.WriteString("\nNear-duplicate categories (merge the smaller into the larger with lattice_merge):\n")
				for _, d := range rep.NearDuplicates {
					fmt.Fprintf(&b, "- %s (%d) ~ %s (%d): %s\n", d.A, d.LinksA, d.B, d.LinksB, d.Reason)
				}
			}
			if len(rep.OutsideAxes) > 0 {
				b.WriteString("\nRoots outside the standard axes (" + strings.Join(store.AxisRoots(), ", ") + "):\n")
				for _, n := range rep.OutsideAxes {
					fmt.Fprintf(&b, "- %s\n", n)
				}
			}
			if len(rep.SingleUse) > 0 {
				b.WriteString("\nSingle-use categories (fine if the topic will recur; otherwise retag into a broader one):\n")
				for _, u := range rep.SingleUse {
					fmt.Fprintf(&b, "- %s → %s  %s\n", u.Category, u.MemoryID, oneLine(u.Summary, 80))
				}
			}
			if len(rep.WeaklyFiled) > 0 {
				b.WriteString("\nMemories without a confirmed placement (classify with lattice_tag):\n")
				for _, w := range rep.WeaklyFiled {
					guess := "unfiled"
					if len(w.Paths) > 0 {
						guess = "guessed: " + strings.Join(w.Paths, ", ")
					}
					fmt.Fprintf(&b, "- %s  %s  (%s)\n", w.MemoryID, oneLine(w.Summary, 80), guess)
				}
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "lattice_merge", Description: "Fold one category into another (moving all its links) to tidy near-duplicates like project/db and project/database.",
		Annotations: writes("Merge categories", false)},
		func(a mergeArgs) (string, string) { return a.From + " → " + a.To, "" },
		func(ctx context.Context, a mergeArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			if err := c.MergeCategory(ctx, a.From, a.To); err != nil {
				return "", err
			}
			return "Merged " + store.NormalizePath(a.From) + " into " + store.NormalizePath(a.To) + ".", nil
		})

	addTool(s, &mcp.Tool{Name: "captures_pending",
		Description: "List conversations memtree's hooks captured that still lack a real summary (newest first). For each, read it with capture_transcript and save a summary with memory_save kind=session and the session_id.",
		Annotations: readOnly("Pending captures")},
		func(a projectArgs) (string, string) { return "", a.Project },
		func(ctx context.Context, a projectArgs) (string, error) {
			list, err := c.Capture.Pending(ctx, s.resolve(a.Project), a.AllProjects, 20)
			if err != nil {
				return "", err
			}
			if len(list) == 0 {
				return "No captured sessions are waiting for a summary.", nil
			}
			var b strings.Builder
			for _, cs := range list {
				fmt.Fprintf(&b, "- %s  %s  %d turns  %s  %s\n", cs.ID, cs.Harness, cs.Turns, core.Ago(c.DB.Now().Sub(cs.LastAt)), scopeName(cs.Project))
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "capture_transcript", Description: "Read the captured turns of one session so you can summarise it.",
		Annotations: readOnly("Read capture")},
		func(a sessionArgs) (string, string) { return a.SessionID, "" },
		func(ctx context.Context, a sessionArgs) (string, error) {
			turns, err := c.Capture.Turns(ctx, a.SessionID)
			if err != nil {
				return "", err
			}
			if len(turns) == 0 {
				return "", fmt.Errorf("no captured turns for session %s", a.SessionID)
			}
			var b strings.Builder
			for _, t := range turns {
				fmt.Fprintf(&b, "[%s] %s\n", t.Role, t.Text)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "consolidation_candidates",
		Description: "Find clusters of old, rarely used, related memories that should be distilled into one. For each cluster, write a gist and call memory_compress. memtree only proposes; you write the gist.",
		Annotations: readOnly("Consolidation candidates")},
		func(a projectArgs) (string, string) { return "", a.Project },
		func(ctx context.Context, a projectArgs) (string, error) {
			cl, err := c.Memory.Candidates(ctx, c.Config.Get().Tiers, s.resolve(a.Project), a.AllProjects)
			if err != nil {
				return "", err
			}
			if len(cl) == 0 {
				return "Nothing to consolidate yet.", nil
			}
			var b strings.Builder
			for i, k := range cl {
				fmt.Fprintf(&b, "Cluster %d (%d memories, shared: %s)\n", i+1, len(k.Records), orNone(strings.Join(k.Shared, ", ")))
				for _, r := range k.Records {
					fmt.Fprintf(&b, "  %s  %s  %s\n", r.ID, core.Ago(c.DB.Now().Sub(r.CreatedAt)), oneLine(r.Summary, 140))
				}
			}
			return strings.TrimRight(b.String(), "\n"), nil
		})

	addTool(s, &mcp.Tool{Name: "memory_compress",
		Description: "Replace several related old memories with one distilled semantic memory. The originals are superseded, not deleted. The gist inherits the highest importance and weakest provenance of its sources, so you cannot inflate them. Pinned memories are refused.",
		Annotations: writes("Compress memories", false)},
		func(a compressArgs) (string, string) { return fmt.Sprintf("%d memories", len(a.IDs)), "" },
		func(ctx context.Context, a compressArgs) (string, error) {
			if err := c.AgentWritable(); err != nil {
				return "", err
			}
			r, err := c.Compress(ctx, a.IDs, store.MemoryRecord{
				Summary: a.Summary, Keywords: a.Keywords, Decisions: a.Decisions, OpenLoops: a.OpenLoops,
				LatticePaths: a.LatticePaths, Harness: "mcp",
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Saved %s (semantic) superseding %d memories.", r.ID, len(a.IDs)), nil
		})
}

// categoriesLine reports the lattice categories a query fired.
func categoriesLine(cats []string) string {
	if len(cats) == 0 {
		return ""
	}
	return "\nFired categories: " + strings.Join(cats, ", ")
}

// scopeName names a project scope for display ("global" for "").
func scopeName(project string) string {
	if project == "" {
		return "global"
	}
	return project
}

// oneLine collapses whitespace and shortens s to n runes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// pathAdvice returns lattice warnings for paths as text to append to a reply,
// or "" when the paths look fine. It must run before the paths are created.
func (s *Server) pathAdvice(ctx context.Context, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	warns, err := s.core.Lattice.CheckPaths(ctx, paths)
	if err != nil || len(warns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nLattice advice:")
	for _, w := range warns {
		b.WriteString("\n- " + w.Message)
		if w.Suggest != "" {
			b.WriteString(" (retag with lattice_tag, or lattice_merge the two)")
		}
	}
	return b.String()
}

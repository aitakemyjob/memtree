package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/core"
)

type harness struct {
	t    *testing.T
	core *core.Service
	cs   *mcp.ClientSession
	repo string
	now  *time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	cfg, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	clock := &now
	svc, err := core.Open(cfg, ":memory:", "test", func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New(svc).MCP().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &harness{t: t, core: svc, cs: cs, repo: repo, now: clock}
}

func (h *harness) call(name string, args map[string]any) (string, bool) {
	h.t.Helper()
	res, err := h.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		h.t.Fatalf("%s: protocol error: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

func (h *harness) must(name string, args map[string]any, wants ...string) string {
	h.t.Helper()
	out, isErr := h.call(name, args)
	if isErr {
		h.t.Fatalf("%s returned tool error: %s", name, out)
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			h.t.Errorf("%s output missing %q:\n%s", name, w, out)
		}
	}
	return out
}

func (h *harness) fails(name string, args map[string]any, want string) {
	h.t.Helper()
	out, isErr := h.call(name, args)
	if !isErr || !strings.Contains(out, want) {
		h.t.Errorf("%s: want error containing %q, got isErr=%v %q", name, want, isErr, out)
	}
}

func idFrom(t *testing.T, out string) string {
	t.Helper()
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "mem_") {
			return strings.TrimRight(f, ".,)")
		}
	}
	t.Fatalf("no id in %q", out)
	return ""
}

func TestSaveRecallRoundTrip(t *testing.T) {
	h := newHarness(t)
	out := h.must("memory_save", map[string]any{
		"summary": "We chose SQLite with FTS5 over Postgres because memtree must run with zero setup.",
		"project": h.repo, "kind": "decision", "importance": 1.5,
		"lattice_paths": []string{"project/decisions/storage"},
		"decisions":     []string{"SQLite FTS5, not Postgres"},
	}, "Saved mem_", "decision")
	id := idFrom(t, out)
	rec := h.must("recall", map[string]any{"query": "why sqlite storage", "project": h.repo},
		id, "FTS5", "lattice: project/decisions/storage", "via")
	if !strings.Contains(rec, "Fired categories") {
		t.Errorf("recall should report fired categories:\n%s", rec)
	}
	h.must("recall", map[string]any{"query": "completely unrelated gardening", "project": h.repo}, "No matching memories")
	h.must("memory_get", map[string]any{"id": id}, "decisions: SQLite FTS5")
	h.must("memory_list", map[string]any{"project": h.repo}, id)
}

func TestProjectScopeAndGlobal(t *testing.T) {
	h := newHarness(t)
	other := t.TempDir()
	os.MkdirAll(filepath.Join(other, ".git"), 0o755)
	h.must("memory_save", map[string]any{"summary": "alpha convention: snake_case", "project": h.repo})
	h.must("memory_save", map[string]any{"summary": "beta convention: camelCase", "project": other})
	h.must("memory_save", map[string]any{"summary": "global convention: write tests first", "global": true})
	out := h.must("recall", map[string]any{"query": "convention", "project": h.repo}, "snake_case", "write tests first")
	if strings.Contains(out, "camelCase") {
		t.Error("other project leaked into recall")
	}
	h.must("recall", map[string]any{"query": "convention", "all_projects": true}, "snake_case", "camelCase")
}

func TestSinceAndRelativeTimes(t *testing.T) {
	h := newHarness(t)
	h.must("memory_save", map[string]any{"summary": "recent thing about caching", "project": h.repo})
	h.must("recall", map[string]any{"query": "caching", "project": h.repo, "since": "7d"}, "recent thing")
	h.must("recall", map[string]any{"query": "caching", "project": h.repo, "until": "7d"}, "No matching")
	h.fails("recall", map[string]any{"query": "caching", "since": "last tuesday"}, "can't read time")
}

func TestLifecycleTools(t *testing.T) {
	h := newHarness(t)
	a := idFrom(t, h.must("memory_save", map[string]any{"summary": "deploys use blue green", "project": h.repo}))
	b := idFrom(t, h.must("memory_save", map[string]any{"summary": "deploys now use canary releases", "project": h.repo}))
	h.must("memory_supersede", map[string]any{"id": a, "by": b}, "superseded")
	out := h.must("recall", map[string]any{"query": "deploys", "project": h.repo})
	if strings.Contains(out, "blue green") || !strings.Contains(out, "canary") {
		t.Errorf("superseded memory should drop out:\n%s", out)
	}
	h.must("recall", map[string]any{"query": "deploys", "project": h.repo, "include_superseded": true}, "blue green", "superseded by "+b)
	h.must("memory_pin", map[string]any{"id": b}, "Pinned")
	h.must("memory_get", map[string]any{"id": b}, "pinned")
	h.must("memory_archive", map[string]any{"id": b}, "Archived")
	h.must("recall", map[string]any{"query": "canary", "project": h.repo}, "No matching")
	h.must("memory_archive", map[string]any{"id": b, "undo": true}, "Restored")
	h.must("memory_update", map[string]any{"id": b, "summary": "deploys use canary then full rollout"}, "Updated")
	h.must("recall", map[string]any{"query": "rollout", "project": h.repo}, "full rollout")
	h.fails("memory_supersede", map[string]any{"id": b, "by": b}, "itself")
	h.fails("memory_get", map[string]any{"id": "mem_missing"}, "not found")
}

func TestClassificationFlow(t *testing.T) {
	h := newHarness(t)
	h.must("lattice_browse", nil, "project", "knowledge")
	out := h.must("memory_save", map[string]any{"summary": "terraform state lives in s3", "project": h.repo}, "unclassified")
	id := idFrom(t, out)
	h.must("lattice_untagged", nil, id)
	h.must("lattice_tag", map[string]any{"id": id, "paths": []string{"knowledge/tools/terraform"}}, "Tagged")
	h.must("lattice_untagged", nil, "Nothing is waiting")
	h.must("lattice_browse", map[string]any{"path": "knowledge/tools", "items": true}, "knowledge/tools/terraform", id)
	h.must("lattice_browse", map[string]any{"path": "knowledge/tools/terraform", "items": true}, id)
	h.must("lattice_activate", map[string]any{"text": "terraform"}, "knowledge/tools/terraform", id)
	h.must("lattice_activate", map[string]any{"text": "zzzz"}, "Nothing in the lattice")
	h.must("lattice_browse", map[string]any{"path": "no/such"}, "No such category")
	h.must("memory_save", map[string]any{"summary": "tfstate lock table", "project": h.repo, "lattice_paths": []string{"knowledge/tools/tf"}})
	h.must("lattice_merge", map[string]any{"from": "knowledge/tools/tf", "to": "knowledge/tools/terraform"}, "Merged")
	h.fails("lattice_merge", map[string]any{"from": "a", "to": "a"}, "different")
}

func TestLintAndPathAdvice(t *testing.T) {
	h := newHarness(t)
	h.must("lattice_lint", nil, "tidy")
	h.must("memory_save", map[string]any{"summary": "migrations run on boot", "project": h.repo, "lattice_paths": []string{"project/database"}}, "Saved")
	out := h.must("memory_save", map[string]any{"summary": "the db pool is capped at 10", "project": h.repo, "lattice_paths": []string{"project/db", "weird/root"}},
		"Lattice advice", "resembles existing \"project/database\"", "not a standard axis")
	id := idFrom(t, out)
	h.must("memory_save", map[string]any{"summary": "clean one", "project": h.repo, "lattice_paths": []string{"project/database"}}) // no advice
	lint := h.must("lattice_lint", nil, "Near-duplicate", "project/database", "project/db", "Roots outside", "weird")
	if !strings.Contains(lint, "lattice_merge") {
		t.Errorf("lint should say how to fix: %s", lint)
	}
	h.must("lattice_merge", map[string]any{"from": "project/db", "to": "project/database"}, "Merged")
	tag := h.must("lattice_tag", map[string]any{"id": id, "paths": []string{"project/database"}}, "Tagged")
	if strings.Contains(tag, "Lattice advice") {
		t.Errorf("an existing path should draw no advice: %s", tag)
	}
	h.must("lattice_tag", map[string]any{"id": id, "paths": []string{"project/databse"}}, "resembles existing")
}

func TestCapturesAndSessionSummary(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	proj := core.ResolveProject(h.repo)
	h.core.CaptureTurn(ctx, "claude-code", "sess1", proj, "user", "speed up the importer")
	h.core.CaptureTurn(ctx, "claude-code", "sess1", proj, "assistant", "batched inserts; 8x faster")
	h.core.CloseSession(ctx, "sess1", "exit")
	h.must("captures_pending", map[string]any{"project": h.repo}, "sess1", "2 turns")
	h.must("capture_transcript", map[string]any{"session_id": "sess1"}, "[user] speed up the importer", "[assistant] batched inserts")
	h.must("memory_save", map[string]any{"summary": "Importer sped up 8x via batched inserts", "project": h.repo, "kind": "session", "session_id": "sess1"})
	h.must("captures_pending", map[string]any{"project": h.repo}, "No captured sessions")
	h.fails("capture_transcript", map[string]any{"session_id": "nope"}, "no captured turns")
	h.fails("memory_save", map[string]any{"summary": "x", "kind": "capture"}, "reserved")
}

func TestConsolidationTools(t *testing.T) {
	h := newHarness(t)
	h.must("consolidation_candidates", map[string]any{"project": h.repo}, "Nothing to consolidate")
	ids := []string{}
	for _, s := range []string{"redis cache eviction lru", "redis cache sizing two gigabytes"} {
		ids = append(ids, idFrom(t, h.must("memory_save", map[string]any{"summary": s, "project": h.repo, "keywords": []string{"redis", "cache"}})))
	}
	// Age the clock past the episodic threshold.
	h.age(100 * time.Hour)
	h.must("consolidation_candidates", map[string]any{"project": h.repo}, "Cluster 1", ids[0], ids[1])
	out := h.must("memory_compress", map[string]any{"ids": ids, "summary": "Redis cache: LRU eviction, 2GB"}, "superseding 2")
	gist := idFrom(t, out)
	h.must("memory_get", map[string]any{"id": gist}, "semantic", "distilled from 2")
	h.fails("memory_compress", map[string]any{"ids": ids, "summary": "again"}, "already superseded")
}

func (h *harness) age(d time.Duration) { *h.now = h.now.Add(d) }

func TestAgentWritesSwitch(t *testing.T) {
	h := newHarness(t)
	h.core.UpdateSettings(func(s *config.Settings) { s.AgentWrites = false })
	h.fails("memory_save", map[string]any{"summary": "nope"}, "agent writes are disabled")
	h.fails("memory_archive", map[string]any{"id": "mem_x"}, "Settings")
	h.must("recall", map[string]any{"query": "anything"}, "No matching") // reads still work
}

func TestActivityFeedRecordsCalls(t *testing.T) {
	h := newHarness(t)
	h.must("recall", map[string]any{"query": "x marks"})
	h.call("memory_get", map[string]any{"id": "mem_missing"})
	var recall, failed bool
	for _, a := range h.core.ActivityLog() {
		recall = recall || a.Tool == "recall" && a.Harness == "test-agent" && a.Source == "mcp"
		failed = failed || a.Tool == "memory_get" && a.Error != ""
	}
	if !recall || !failed {
		t.Errorf("activity %+v", h.core.ActivityLog())
	}
}

func TestParseWhen(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"30m": now.Add(-30 * time.Minute), "24h": now.Add(-24 * time.Hour), "2w": now.Add(-14 * 24 * time.Hour),
		"2026-06-01": time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	} {
		got, err := parseWhen(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: %v %v want %v", in, got, err, want)
		}
	}
	if got, _ := parseWhen("", now); got != nil {
		t.Error("empty means no bound")
	}
}

// TestToolsAreDocumented keeps docs/MCP.md honest: every registered tool
// must appear there by name.
func TestToolsAreDocumented(t *testing.T) {
	h := newHarness(t)
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "MCP.md"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) < 15 {
		t.Errorf("only %d tools registered", len(res.Tools))
	}
	for _, tool := range res.Tools {
		if !strings.Contains(string(doc), "`"+tool.Name+"`") {
			t.Errorf("tool %s is missing from docs/MCP.md", tool.Name)
		}
		if len(tool.Description) < 40 {
			t.Errorf("tool %s needs a description written for an LLM", tool.Name)
		}
	}
}

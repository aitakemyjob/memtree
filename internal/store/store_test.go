package store

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

// clock is a settable test clock.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

type fixture struct {
	db  *DB
	lat *Lattice
	mem *Memory
	cap *Captures
	clk *clock
	ctx context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	clk := &clock{t: t0}
	db, err := Open(":memory:", clk.now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	lat := NewLattice(db)
	mem := NewMemory(db, lat)
	return &fixture{db: db, lat: lat, mem: mem, cap: NewCaptures(db, mem), clk: clk, ctx: context.Background()}
}

func hoursAgo(h float64) time.Time { return t0.Add(-time.Duration(h * float64(time.Hour))) }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// ---- activation ----

func TestActivationRaw(t *testing.T) {
	if !math.IsInf(ActivationRaw(nil, 0.5, t0), -1) {
		t.Error("empty log should be -Inf")
	}
	if !near(ActivationRaw([]time.Time{t0}, 0.5, t0), 0) {
		t.Error("a just-now access clamps to 1h → ln(1)=0")
	}
	if ActivationRaw([]time.Time{hoursAgo(1)}, 0.5, t0) <= ActivationRaw([]time.Time{hoursAgo(100)}, 0.5, t0) {
		t.Error("older access should be lower")
	}
	one := ActivationRaw([]time.Time{hoursAgo(10)}, 0.5, t0)
	two := ActivationRaw([]time.Time{hoursAgo(10), hoursAgo(10)}, 0.5, t0)
	if two <= one {
		t.Error("accesses should accumulate")
	}
}

func TestActivationScore(t *testing.T) {
	p := EpisodicParams
	if ActivationScore(nil, p, t0) != DefaultFloor {
		t.Error("empty log should be the floor")
	}
	s := ActivationScore([]time.Time{t0, t0, t0}, p, t0)
	if s < DefaultFloor || s > 1 {
		t.Errorf("score %v out of [floor,1]", s)
	}
	if ActivationScore([]time.Time{hoursAgo(1)}, p, t0) < ActivationScore([]time.Time{hoursAgo(500)}, p, t0) {
		t.Error("recency should score higher")
	}
	var log []time.Time
	for i := 0; i < AccessLogCap+5; i++ {
		log = TouchAccessLog(log, hoursAgo(float64(i)))
	}
	if len(log) != AccessLogCap {
		t.Errorf("log len %d, want %d", len(log), AccessLogCap)
	}
}

func TestRelevanceIsRelativeToBest(t *testing.T) {
	if relevance(-2, -4) != 0.5 || relevance(-4, -4) != 1 {
		t.Error("relative to best")
	}
	if relevance(-1e-6, -1e-6) != 1 || relevance(0, 0) != 1 {
		t.Error("degenerate small-store ranks still score 1")
	}
}

func TestNewMemoryStartsWarmAndCools(t *testing.T) {
	f := newFixture(t)
	r, _ := f.mem.Save(f.ctx, rec("fresh note about queues", nil))
	if len(r.AccessLog) != 1 || !r.AccessLog[0].Equal(r.CreatedAt) {
		t.Fatalf("creation should seed the access log: %v", r.AccessLog)
	}
	warm := f.mem.ActivationOf(r)
	f.clk.advance(30 * 24 * time.Hour)
	if cool := f.mem.ActivationOf(r); !(warm > cool && cool > DefaultFloor) {
		t.Errorf("activation should decay with age: %v → %v", warm, cool)
	}
	hits, _ := f.mem.Search(f.ctx, Query{Text: "queues", Project: "/repo/a"})
	if hits[0].Score.BM25Normalized != 1 {
		t.Errorf("a lone match has full relevance even in a tiny store: %+v", hits[0].Score)
	}
}

// ---- lattice ----

func TestEnsureNodeChainAndNormalize(t *testing.T) {
	f := newFixture(t)
	leaf, err := f.lat.EnsureNode(f.ctx, "self/memory/conflation")
	if err != nil || leaf != "self/memory/conflation" {
		t.Fatalf("leaf %q err %v", leaf, err)
	}
	f.lat.EnsureNode(f.ctx, "self/memory/conflation")
	ids, _ := f.lat.AllNodeIDs(f.ctx)
	if strings.Join(ids, ",") != "self,self/memory,self/memory/conflation" {
		t.Errorf("ids %v", ids)
	}
	if got := NormalizePath("Self / Reflection!! / "); got != "self/reflection" {
		t.Errorf("normalize %q", got)
	}
	if _, err := f.lat.EnsureNode(f.ctx, " / !! "); err == nil {
		t.Error("empty path should error")
	}
}

func TestLinkReinforceDecay(t *testing.T) {
	f := newFixture(t)
	f.lat.Link(f.ctx, "memory", "m1", "knowledge/domain", 0.5, "", "")
	links, _ := f.lat.LinksFor(f.ctx, "knowledge/domain")
	if !near(links[0].Weight, 0.5) {
		t.Fatalf("weight %v", links[0].Weight)
	}
	f.lat.Reinforce(f.ctx, "memory", "m1", 0.3)
	links, _ = f.lat.LinksFor(f.ctx, "knowledge/domain")
	if !near(links[0].Weight, 0.8) {
		t.Errorf("reinforced %v", links[0].Weight)
	}
	f.lat.Reinforce(f.ctx, "memory", "m1", 0.5)
	links, _ = f.lat.LinksFor(f.ctx, "knowledge/domain")
	if !near(links[0].Weight, 1) {
		t.Errorf("clamped %v", links[0].Weight)
	}

	g := newFixture(t)
	g.lat.Link(g.ctx, "journal", "j1", "affect/curiosity", 0.4, "", "")
	g.clk.advance(48 * time.Hour)
	if n, _ := g.lat.DecayLinks(g.ctx, 1, 0.08); n != 0 {
		t.Errorf("dropped %d at 2 days", n)
	}
	links, _ = g.lat.LinksFor(g.ctx, "affect/curiosity")
	if math.Abs(links[0].Weight-0.1) > 1e-5 {
		t.Errorf("half-life weight %v want 0.1", links[0].Weight)
	}
	g.clk.advance(8 * 24 * time.Hour)
	if n, _ := g.lat.DecayLinks(g.ctx, 1, 0.08); n != 1 {
		t.Errorf("should drop faded link, dropped %d", n)
	}
	ids, _ := g.lat.AllNodeIDs(g.ctx)
	if !contains(ids, "affect/curiosity") {
		t.Error("node must survive link decay")
	}
}

func TestSpreadingActivation(t *testing.T) {
	f := newFixture(t)
	f.lat.Link(f.ctx, "thought", "t-refl", "self/reflection", 0.9, "", "")
	f.lat.Link(f.ctx, "memory", "m-self", "self/memory", 0.9, "", "")
	f.lat.Link(f.ctx, "reflection", "r-self", "self", 0.9, "", "")
	hits, err := f.lat.Activate(f.ctx, []string{"self"}, ActivateOptions{Threshold: 0.01, Max: 10})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]float64{}
	for _, h := range hits {
		by[h.ItemID] = h.Score
	}
	if by["r-self"] <= 0 || by["t-refl"] <= 0 || by["m-self"] <= 0 {
		t.Fatalf("missing hits %v", by)
	}
	if by["r-self"] <= by["t-refl"] {
		t.Error("seeded item should outscore a child one hop away")
	}

	g := newFixture(t)
	g.lat.Link(g.ctx, "thought", "a", "self/reflection", 0.9, "", "")
	g.lat.Link(g.ctx, "thought", "b", "self/reflection", 0.2, "", "")
	strong, _ := g.lat.Activate(g.ctx, []string{"self/reflection"}, ActivateOptions{Threshold: 0.5, Max: 10})
	if len(strong) != 1 || strong[0].ItemID != "a" {
		t.Errorf("threshold: %v", strong)
	}
	capped, _ := g.lat.Activate(g.ctx, []string{"self/reflection"}, ActivateOptions{Threshold: 0.01, Max: 1})
	if len(capped) != 1 || capped[0].ItemID != "a" {
		t.Errorf("max: %v", capped)
	}
	none, _ := g.lat.Activate(g.ctx, []string{"nonexistent/node"}, ActivateOptions{Threshold: 0.01, Max: 10})
	if len(none) != 0 {
		t.Error("unknown seed should surface nothing")
	}

	h := newFixture(t)
	h.lat.Link(h.ctx, "thought", "t", "self/reflection", 0.9, "", "")
	h.lat.Link(h.ctx, "memory", "m", "self/memory", 0.9, "", "")
	h.lat.Link(h.ctx, "knowledge", "k", "knowledge/domain", 0.9, "", "")
	r, _ := h.lat.Activate(h.ctx, []string{"self", "knowledge/domain"}, ActivateOptions{Threshold: 0.01, Max: 10, ItemTypes: []string{"memory"}, AxisRoots: []string{"self"}})
	if len(r) != 1 || r[0].ItemID != "m" {
		t.Errorf("itemTypes+axisRoots: %v", r)
	}
}

func TestNodeTimeDecayLowersActivation(t *testing.T) {
	f := newFixture(t)
	f.lat.Link(f.ctx, "memory", "m", "project/build", 0.9, "", "")
	fresh, _ := f.lat.Activate(f.ctx, []string{"project/build"}, ActivateOptions{Threshold: 0, Max: 1})
	f.clk.advance(90 * 24 * time.Hour)
	stale, _ := f.lat.Activate(f.ctx, []string{"project/build"}, ActivateOptions{Threshold: 0, Max: 1})
	if stale[0].Score >= fresh[0].Score {
		t.Errorf("untouched node should cool: %v → %v", fresh[0].Score, stale[0].Score)
	}
}

func TestSeedAndSignals(t *testing.T) {
	f := newFixture(t)
	f.lat.SeedAxes(f.ctx)
	f.lat.SeedAxes(f.ctx)
	ids, _ := f.lat.AllNodeIDs(f.ctx)
	for _, s := range AxisSeeds {
		if !contains(ids, s) {
			t.Errorf("missing seed %s", s)
		}
	}
	f.lat.EnsureNode(f.ctx, "knowledge/tools/sqlite")
	got, _ := f.lat.SignalCategories(f.ctx, "how does the sqlite fts index work?")
	if !contains(got, "knowledge/tools/sqlite") {
		t.Errorf("signals %v", got)
	}
	empty, _ := f.lat.SignalCategories(f.ctx, "hi, how's it going")
	if len(empty) != 0 {
		t.Errorf("contentless text fired %v", empty)
	}
	sug, _ := f.lat.SuggestPaths(f.ctx, "sqlite tools", 3)
	if len(sug) == 0 || sug[0] != "knowledge/tools/sqlite" || contains(sug, "knowledge/tools") {
		t.Errorf("suggest should prefer deepest and drop covered parents: %v", sug)
	}
}

func TestMergeAndPrune(t *testing.T) {
	f := newFixture(t)
	f.lat.Link(f.ctx, "memory", "m1", "project/db", 0.6, "a", "")
	f.lat.Link(f.ctx, "memory", "m2", "project/database", 0.7, "b", "")
	if err := f.lat.MergeNode(f.ctx, "project/db", "project/database"); err != nil {
		t.Fatal(err)
	}
	links, _ := f.lat.LinksFor(f.ctx, "project/database")
	if len(links) != 2 {
		t.Errorf("merged links %d", len(links))
	}
	ids, _ := f.lat.AllNodeIDs(f.ctx)
	if contains(ids, "project/db") {
		t.Error("empty source node should be removed")
	}
	f.lat.EnsureNode(f.ctx, "scratch/a/b")
	n, _ := f.lat.PruneEmpty(f.ctx, AxisSeeds)
	ids, _ = f.lat.AllNodeIDs(f.ctx)
	if n != 3 || contains(ids, "scratch") {
		t.Errorf("prune removed %d, ids %v", n, ids)
	}
}

// ---- memory ----

func rec(summary string, mod func(*MemoryRecord)) MemoryRecord {
	r := MemoryRecord{Summary: summary, Project: "/repo/a"}
	if mod != nil {
		mod(&r)
	}
	return r
}

func TestSaveAndSearch(t *testing.T) {
	f := newFixture(t)
	r, err := f.mem.Save(f.ctx, rec("Chose openWakeWord over Porcupine for wake word detection", func(r *MemoryRecord) {
		r.Importance = 1.5
		r.LatticePaths = []string{"project/decisions/audio"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" || r.Tier != TierEpisodic || r.Importance != ImportanceDecision || len(r.Keywords) == 0 {
		t.Errorf("defaults not applied: %+v", r)
	}
	f.mem.Save(f.ctx, rec("Camping trip packing list", nil))
	hits, err := f.mem.Search(f.ctx, Query{Text: "wake word", Project: "/repo/a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Record.ID != r.ID {
		t.Fatalf("hits %+v", hits)
	}
	s := hits[0].Score
	if !near(s.Final, s.BM25Normalized*s.ImportanceOffset*s.Activation*s.ProjectBoost) || s.ProjectBoost != ProjectBoost {
		t.Errorf("score decomposition %+v", s)
	}
	if !contains(hits[0].Record.LatticePaths, "project/decisions/audio") {
		t.Errorf("paths %v", hits[0].Record.LatticePaths)
	}
	if _, err := f.mem.Save(f.ctx, rec("   ", nil)); err == nil {
		t.Error("empty summary should fail")
	}
	if none, _ := f.mem.Search(f.ctx, Query{Text: "!!"}); len(none) != 0 {
		t.Error("punctuation-only query should match nothing")
	}
}

func TestImportanceAndTouchRank(t *testing.T) {
	f := newFixture(t)
	low, _ := f.mem.Save(f.ctx, rec("deploy pipeline uses github actions", nil))
	high, _ := f.mem.Save(f.ctx, rec("deploy pipeline broke prod once: always pin the runner image", func(r *MemoryRecord) { r.Importance = 2.5 }))
	hits, _ := f.mem.Search(f.ctx, Query{Text: "deploy pipeline", Project: "/repo/a"})
	if hits[0].Record.ID != high.ID {
		t.Error("importance should rank the lesson first")
	}
	before := hits[1].Score.Activation
	f.mem.Touch(f.ctx, []string{low.ID, low.ID, low.ID})
	hits, _ = f.mem.Search(f.ctx, Query{Text: "deploy pipeline", Project: "/repo/a"})
	for _, h := range hits {
		if h.Record.ID == low.ID && h.Score.Activation <= before {
			t.Error("touch should raise activation")
		}
	}
}

func TestScopingTimeAndLifecycle(t *testing.T) {
	f := newFixture(t)
	a, _ := f.mem.Save(f.ctx, rec("redis cache eviction policy", nil))
	g, _ := f.mem.Save(f.ctx, rec("redis prefers lowercase keys globally", func(r *MemoryRecord) { r.Project = "" }))
	b, _ := f.mem.Save(f.ctx, rec("redis cluster in project b", func(r *MemoryRecord) { r.Project = "/repo/b" }))
	ids := func(q Query) []string {
		hits, _ := f.mem.Search(f.ctx, q)
		var out []string
		for _, h := range hits {
			out = append(out, h.Record.ID)
		}
		return out
	}
	inA := ids(Query{Text: "redis", Project: "/repo/a"})
	if !contains(inA, a.ID) || !contains(inA, g.ID) || contains(inA, b.ID) {
		t.Errorf("project scope %v", inA)
	}
	if len(ids(Query{Text: "redis", AllProjects: true})) != 3 {
		t.Error("all projects should see everything")
	}
	f.clk.advance(48 * time.Hour)
	late, _ := f.mem.Save(f.ctx, rec("redis upgrade to 8", nil))
	cut := t0.Add(24 * time.Hour)
	if got := ids(Query{Text: "redis", Project: "/repo/a", After: &cut}); len(got) != 1 || got[0] != late.ID {
		t.Errorf("after filter %v", got)
	}
	if got := ids(Query{Text: "redis", Project: "/repo/a", Before: &cut}); contains(got, late.ID) {
		t.Errorf("before filter %v", got)
	}
	f.mem.Supersede(f.ctx, a.ID, late.ID)
	if contains(ids(Query{Text: "redis", Project: "/repo/a"}), a.ID) {
		t.Error("superseded excluded by default")
	}
	f.mem.Unsupersede(f.ctx, a.ID)
	f.mem.Archive(f.ctx, a.ID)
	if contains(ids(Query{Text: "redis", Project: "/repo/a"}), a.ID) {
		t.Error("archived excluded")
	}
	f.mem.Restore(f.ctx, a.ID)
	if !contains(ids(Query{Text: "redis", Project: "/repo/a"}), a.ID) {
		t.Error("restored should be back")
	}
	if err := f.mem.Supersede(f.ctx, a.ID, a.ID); err == nil {
		t.Error("self-supersede should fail")
	}
	f.mem.Delete(f.ctx, b.ID)
	if _, err := f.mem.Get(f.ctx, b.ID); err == nil {
		t.Error("deleted record should be gone")
	}
}

func TestUpdateReindexesAndRelinks(t *testing.T) {
	f := newFixture(t)
	r, _ := f.mem.Save(f.ctx, rec("uses postgres", nil))
	upd, err := f.mem.Update(f.ctx, r.ID, func(x *MemoryRecord) { x.Summary = "migrated to cockroachdb"; x.Keywords = nil }, []string{"project/architecture/db"})
	if err != nil {
		t.Fatal(err)
	}
	if hits, _ := f.mem.Search(f.ctx, Query{Text: "postgres", Project: "/repo/a"}); len(hits) != 0 {
		t.Error("old text should be unindexed")
	}
	if hits, _ := f.mem.Search(f.ctx, Query{Text: "cockroachdb", Project: "/repo/a"}); len(hits) != 1 {
		t.Error("new text should be indexed")
	}
	if len(upd.LatticePaths) != 1 || upd.LatticePaths[0] != "project/architecture/db" {
		t.Errorf("paths %v", upd.LatticePaths)
	}
	if n, _ := f.lat.UntaggedCount(f.ctx); n != 0 {
		t.Error("human classification should clear the untagged queue")
	}
}

func TestUnclassifiedSaveGetsProvisionalPlacement(t *testing.T) {
	f := newFixture(t)
	f.lat.SeedAxes(f.ctx)
	f.lat.EnsureNode(f.ctx, "knowledge/tools/docker")
	r, _ := f.mem.Save(f.ctx, rec("docker compose needs the --wait flag in CI", nil))
	if !contains(r.LatticePaths, "knowledge/tools/docker") {
		t.Errorf("provisional paths %v", r.LatticePaths)
	}
	links, _ := f.lat.LinksForItem(f.ctx, ItemMemory, r.ID)
	if links[0].Source != SourceAuto || !near(links[0].Weight, AutoLinkWeight) {
		t.Errorf("link %+v", links[0])
	}
	if n, _ := f.lat.UntaggedCount(f.ctx); n != 1 {
		t.Error("unclassified record should be queued")
	}
}

func TestPinnedNeverFades(t *testing.T) {
	f := newFixture(t)
	r, _ := f.mem.Save(f.ctx, rec("always run make lint before pushing", nil))
	f.mem.SetPinned(f.ctx, r.ID, true)
	f.clk.advance(365 * 24 * time.Hour)
	hits, _ := f.mem.Search(f.ctx, Query{Text: "lint", Project: "/repo/a"})
	if hits[0].Score.Activation != 1 {
		t.Errorf("pinned activation %v", hits[0].Score.Activation)
	}
	got, _ := f.mem.Get(f.ctx, r.ID)
	if ShouldCompress(got, 0, f.db.Now(), DefaultTierConfig()) {
		t.Error("pinned must not compress")
	}
}

// ---- consolidation ----

func TestShouldCompress(t *testing.T) {
	cfg := DefaultTierConfig()
	ep := MemoryRecord{Tier: TierEpisodic, CreatedAt: hoursAgo(100)}
	if !ShouldCompress(ep, 0.2, t0, cfg) {
		t.Error("old cool episodic compresses")
	}
	if ShouldCompress(ep, 0.9, t0, cfg) {
		t.Error("hot records stay")
	}
	if ShouldCompress(MemoryRecord{Tier: TierEpisodic, CreatedAt: hoursAgo(10)}, 0.2, t0, cfg) {
		t.Error("young records stay")
	}
	if ShouldCompress(MemoryRecord{Tier: TierSemantic, CreatedAt: hoursAgo(100)}, 0.2, t0, cfg) {
		t.Error("semantic waits for the compact age")
	}
	if !ShouldCompress(MemoryRecord{Tier: TierSemantic, CreatedAt: hoursAgo(800)}, 0.2, t0, cfg) {
		t.Error("old semantic recompresses")
	}
}

func TestClusterRecords(t *testing.T) {
	r := func(p string, kw ...string) MemoryRecord { return MemoryRecord{Project: p, Keywords: kw} }
	cs := ClusterRecords([]MemoryRecord{
		r("a", "redis", "cache"), r("a", "redis", "eviction"), r("b", "redis", "cache"), r("a", "camping"),
	}, 2)
	if len(cs) != 1 || len(cs[0]) != 2 {
		t.Errorf("clusters %v", cs)
	}
	if jaccard(nil, nil) != 1 {
		t.Error("empty/empty jaccard is 1 for clustering")
	}
}

func TestCandidatesAndCompress(t *testing.T) {
	f := newFixture(t)
	a, _ := f.mem.Save(f.ctx, rec("redis cache eviction uses allkeys-lru", func(r *MemoryRecord) {
		r.Keywords = []string{"redis", "cache", "eviction"}
		r.Provenance = ProvHuman
		r.LatticePaths = []string{"knowledge/tools/redis"}
	}))
	b, _ := f.mem.Save(f.ctx, rec("redis cache size raised to 2gb", func(r *MemoryRecord) {
		r.Keywords = []string{"redis", "cache", "size"}
		r.Importance = 2.5
		r.Provenance = ProvSource
	}))
	f.mem.Save(f.ctx, rec("unrelated camping note", nil))
	f.clk.advance(100 * time.Hour)
	cands, err := f.mem.Candidates(f.ctx, DefaultTierConfig(), "/repo/a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || len(cands[0].Records) != 2 || !contains(cands[0].Shared, "redis") {
		t.Fatalf("candidates %+v", cands)
	}
	gist, err := f.mem.Compress(f.ctx, []string{a.ID, b.ID}, MemoryRecord{Summary: "Redis cache: allkeys-lru eviction, 2GB", Importance: 1, Provenance: ProvHuman})
	if err != nil {
		t.Fatal(err)
	}
	if gist.Tier != TierSemantic || gist.Importance != ImportanceLesson || gist.Provenance != ProvSource || gist.Project != "/repo/a" {
		t.Errorf("gist rules not enforced: %+v", gist)
	}
	if !contains(gist.LatticePaths, "knowledge/tools/redis") {
		t.Errorf("gist should inherit input paths: %v", gist.LatticePaths)
	}
	old, _ := f.mem.Get(f.ctx, a.ID)
	if old.SupersededBy != gist.ID {
		t.Error("inputs should be superseded, not deleted")
	}
	lin, _ := f.mem.Lineage(f.ctx, gist.ID)
	if len(lin) != 2 {
		t.Errorf("lineage %d", len(lin))
	}
	if _, err := f.mem.Compress(f.ctx, []string{a.ID}, MemoryRecord{Summary: "x"}); err == nil {
		t.Error("already-superseded input should be refused")
	}
}

// ---- captures ----

func TestCaptureLifecycle(t *testing.T) {
	f := newFixture(t)
	f.cap.AddTurn(f.ctx, "claude-code", "s1", "/repo/a", "user", "Fix the flaky websocket reconnect test")
	f.cap.AddTurn(f.ctx, "claude-code", "s1", "/repo/a", "assistant", "Added exponential backoff with jitter; test now passes 50/50 runs.")
	if err := f.cap.AddTurn(f.ctx, "claude-code", "s1", "", "system", "x"); err == nil {
		t.Error("unknown role should fail")
	}
	raw, err := f.cap.Close(f.ctx, "s1", "prompt_input_exit")
	if err != nil || raw == nil {
		t.Fatalf("close: %v %v", raw, err)
	}
	if raw.Kind != KindCapture || !strings.Contains(raw.Summary, "websocket") || !strings.Contains(raw.Summary, "Assistant (last)") {
		t.Errorf("raw capture %+v", raw)
	}
	again, _ := f.cap.Close(f.ctx, "s1", "other")
	if again.ID != raw.ID {
		t.Error("closing twice should refresh the same record")
	}
	pend, _ := f.cap.Pending(f.ctx, "/repo/a", false, 0)
	if len(pend) != 1 || pend[0].Turns != 2 {
		t.Fatalf("pending %+v", pend)
	}
	hits, _ := f.mem.Search(f.ctx, Query{Text: "websocket", Project: "/repo/a"})
	if len(hits) != 1 {
		t.Error("raw capture should be recallable before summary")
	}
	sum, _ := f.mem.Save(f.ctx, rec("Websocket reconnect flake fixed with backoff+jitter", func(r *MemoryRecord) { r.Kind = KindSession; r.SessionID = "s1" }))
	if err := f.cap.MarkSummarized(f.ctx, "s1", sum.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ = f.cap.Pending(f.ctx, "/repo/a", false, 0)
	if len(pend) != 0 {
		t.Error("summarised session should leave the queue")
	}
	old, _ := f.mem.Get(f.ctx, raw.ID)
	if old.SupersededBy != sum.ID {
		t.Error("raw capture should be superseded by the summary")
	}
	if r, _ := f.cap.Close(f.ctx, "missing", ""); r != nil {
		t.Error("closing an unknown session is a no-op")
	}
}

func TestStats(t *testing.T) {
	f := newFixture(t)
	f.mem.Save(f.ctx, rec("one", nil))
	r, _ := f.mem.Save(f.ctx, rec("two", func(r *MemoryRecord) { r.LatticePaths = []string{"a/b"} }))
	f.mem.SetPinned(f.ctx, r.ID, true)
	s, err := f.mem.Stats(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 2 || s.Episodic != 2 || s.Pinned != 1 || s.Untagged != 1 || s.Links != 1 {
		t.Errorf("stats %+v", s)
	}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

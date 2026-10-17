package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/store"
)

type clk struct{ t time.Time }

func (c *clk) now() time.Time { return c.t }

func newSvc(t *testing.T) (*Service, *clk) {
	t.Helper()
	cfg, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &clk{t: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)}
	s, err := Open(cfg, ":memory:", "test", c.now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, c
}

func save(t *testing.T, s *Service, summary string, mod func(*store.MemoryRecord)) store.MemoryRecord {
	t.Helper()
	r := store.MemoryRecord{Summary: summary, Project: "/p"}
	if mod != nil {
		mod(&r)
	}
	out, err := s.Save(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestResolveProject(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "repo", "a", "b"), 0o755)
	real, _ := filepath.EvalSymlinks(root)
	want := filepath.Join(real, "repo")
	if got := ResolveProject(filepath.Join(root, "repo", "a", "b")); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := ResolveProject(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestRecallMergesTextAndLattice(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	// Found only by text: no lattice path involved.
	textOnly := save(t, s, "kubernetes ingress needs the rewrite annotation", nil)
	// Found only by lattice: the query word "deploys" never appears in its text.
	viaLattice := save(t, s, "always tag the release before pushing the image", func(r *store.MemoryRecord) {
		r.LatticePaths = []string{"project/deploys"}
	})
	res, err := s.Recall(ctx, RecallInput{Query: "how do deploys work", Project: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	var found *Recalled
	for i := range res.Memories {
		if res.Memories[i].Record.ID == viaLattice.ID {
			found = &res.Memories[i]
		}
	}
	if found == nil || found.Source != "lattice" || found.Via != "project/deploys" {
		t.Fatalf("lattice route missing: %+v", res)
	}
	if len(res.Categories) == 0 {
		t.Error("recall should report fired categories")
	}
	res, _ = s.Recall(ctx, RecallInput{Query: "kubernetes ingress", Project: "/p"})
	if len(res.Memories) == 0 || res.Memories[0].Record.ID != textOnly.ID || res.Memories[0].Source != "text" {
		t.Errorf("text route: %+v", res)
	}
}

func TestRecallBothSourcesAndReinforce(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	m := save(t, s, "redis eviction policy is allkeys-lru", func(r *store.MemoryRecord) { r.LatticePaths = []string{"knowledge/tools/redis"} })
	res, _ := s.Recall(ctx, RecallInput{Query: "redis eviction", Project: "/p"})
	if len(res.Memories) != 1 || res.Memories[0].Source != "both" {
		t.Fatalf("want both: %+v", res.Memories)
	}
	before := len(mustGet(t, s, m.ID).AccessLog)
	s.Recall(ctx, RecallInput{Query: "redis eviction", Project: "/p", Reinforce: true})
	if len(mustGet(t, s, m.ID).AccessLog) != before+1 {
		t.Error("reinforce should touch the returned memory")
	}
}

func mustGet(t *testing.T, s *Service, id string) store.MemoryRecord {
	t.Helper()
	r, err := s.Memory.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRecallRespectsScopeAndLifecycleOnLatticeRoute(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	other := save(t, s, "alpha secret", func(r *store.MemoryRecord) { r.Project = "/other"; r.LatticePaths = []string{"project/alpha"} })
	gone := save(t, s, "alpha archived", func(r *store.MemoryRecord) { r.LatticePaths = []string{"project/alpha"} })
	s.Archive(ctx, gone.ID)
	res, _ := s.Recall(ctx, RecallInput{Query: "alpha", Project: "/p"})
	for _, m := range res.Memories {
		if m.Record.ID == other.ID || m.Record.ID == gone.ID {
			t.Errorf("leaked %s", m.Record.ID)
		}
	}
	all, _ := s.Recall(ctx, RecallInput{Query: "alpha", AllProjects: true})
	if len(all.Memories) != 1 || all.Memories[0].Record.ID != other.ID {
		t.Errorf("all-projects should see other project but not archived: %+v", all.Memories)
	}
}

func TestSurfaceQuietWhenNothingRelates(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	save(t, s, "wake word detection uses openWakeWord", func(r *store.MemoryRecord) { r.LatticePaths = []string{"project/audio"} })
	for _, p := range []string{"hi, how's it going", "what's the capital of France?", "ok"} {
		sf, err := s.SurfaceForPrompt(ctx, p, "/p")
		if err != nil {
			t.Fatal(err)
		}
		if sf.Text != "" || len(sf.Items) != 0 {
			t.Errorf("%q should be quiet, got %q", p, sf.Text)
		}
	}
	sf, _ := s.SurfaceForPrompt(ctx, "why does the wake word detection lag?", "/p")
	if !strings.Contains(sf.Text, "openWakeWord") || !strings.Contains(sf.Text, "may be outdated") {
		t.Errorf("should surface with caveat: %q", sf.Text)
	}
	if n := len(mustGet(t, s, sf.Items[0].ID).AccessLog); n != 1 { // just the creation stamp
		t.Error("surfacing must not reinforce")
	}
}

func TestSurfaceLatticeThresholdAndAutoLinks(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	// Provisional (auto) links sit below the surfacing threshold: a record
	// with no agent classification must not surface through the lattice alone.
	s.Lattice.EnsureNode(ctx, "knowledge/tools/terraform")
	auto := save(t, s, "state locking needs dynamodb table", nil) // auto-placed only if keywords match; none do
	_ = auto
	s.Lattice.Link(ctx, "memory", auto.ID, "knowledge/tools/terraform", store.AutoLinkWeight, "x", store.SourceAuto)
	sf, _ := s.SurfaceForPrompt(ctx, "terraform", "/p")
	for _, it := range sf.Items {
		if it.ID == auto.ID && strings.Contains(it.Why, "filed under") {
			t.Errorf("auto link should not clear the threshold: %+v", it)
		}
	}
	agent := save(t, s, "terraform workspaces map to environments", func(r *store.MemoryRecord) { r.LatticePaths = []string{"knowledge/tools/terraform"} })
	sf, _ = s.SurfaceForPrompt(ctx, "terraform", "/p")
	if !hasItem(sf, agent.ID) {
		t.Errorf("agent-classified memory should surface: %+v", sf)
	}
}

func hasItem(sf Surface, id string) bool {
	for _, it := range sf.Items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func TestSurfaceSkipsRawCapturesAndCaps(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	s.CaptureTurn(ctx, "claude-code", "s1", "/p", "user", "debugging the zebra parser")
	s.CloseSession(ctx, "s1", "x")
	for i := 0; i < 6; i++ {
		save(t, s, "zebra parser note number "+string(rune('a'+i)), nil)
	}
	sf, _ := s.SurfaceForPrompt(ctx, "zebra parser", "/p")
	if len(sf.Items) > s.Config.Get().MaxSurface {
		t.Errorf("cap exceeded: %d", len(sf.Items))
	}
	for _, it := range sf.Items {
		if mustGet(t, s, it.ID).Kind == store.KindCapture {
			t.Error("raw capture surfaced")
		}
	}
}

func TestAnchor(t *testing.T) {
	s, c := newSvc(t)
	ctx := context.Background()
	empty, _ := s.AnchorFor(ctx, "/p", "")
	if !strings.Contains(empty, "memtree is connected") || strings.Contains(empty, "Last session") {
		t.Errorf("empty anchor: %q", empty)
	}
	save(t, s, "old landmark: never force-push main", func(r *store.MemoryRecord) { r.Importance = 2.5 })
	c.t = c.t.Add(24 * time.Hour)
	save(t, s, "yesterday we split the api package", func(r *store.MemoryRecord) { r.OpenLoops = []string{"write migration notes"} })
	pin := save(t, s, "use tabs in go files", nil)
	s.SetPinned(ctx, pin.ID, true)
	c.t = c.t.Add(2 * time.Hour)
	s.CaptureTurn(ctx, "claude-code", "sx", "/p", "user", "refactor")
	s.CloseSession(ctx, "sx", "exit")
	a, _ := s.AnchorFor(ctx, "/p", "")
	for _, want := range []string{"Last session notes", "split the api package", "Landmark", "force-push", "Pinned", "tabs", "write migration notes", "1 recent session(s)"} {
		if !strings.Contains(a, want) {
			t.Errorf("anchor missing %q:\n%s", want, a)
		}
	}
	if !strings.Contains(a, "Memory upkeep") || !strings.Contains(a, "captures_pending") {
		t.Errorf("the anchor should firmly ask for upkeep:\n%s", a)
	}
	if self, _ := s.AnchorFor(ctx, "/p", "sx"); strings.Contains(self, "recent session(s)") {
		t.Error("the current session must not count as pending")
	}
	if other, _ := s.AnchorFor(ctx, "/elsewhere", ""); strings.Contains(other, "split the api") {
		t.Error("anchor must be project scoped")
	}
}

func TestSaveSessionSummaryRetiresCapture(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	s.CaptureTurn(ctx, "codex", "s9", "/p", "user", "add retries to uploader")
	s.CaptureTurn(ctx, "codex", "s9", "/p", "assistant", "done with jittered retries")
	s.CloseSession(ctx, "s9", "other")
	if p, _ := s.Capture.Pending(ctx, "/p", false, 0); len(p) != 1 {
		t.Fatal("expected a pending capture")
	}
	save(t, s, "Uploader now retries with jitter", func(r *store.MemoryRecord) { r.Kind = store.KindSession; r.SessionID = "s9" })
	if p, _ := s.Capture.Pending(ctx, "/p", false, 0); len(p) != 0 {
		t.Error("summary should clear the pending capture")
	}
}

func TestCaptureSwitchAndAgentWrites(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	s.UpdateSettings(func(x *config.Settings) { x.CaptureHooks = false; x.AgentWrites = false })
	s.CaptureTurn(ctx, "claude-code", "off", "/p", "user", "hello")
	if _, err := s.Capture.Session(ctx, "off"); err == nil {
		t.Error("capture off should record nothing")
	}
	if err := s.AgentWritable(); err != ErrAgentWritesDisabled {
		t.Errorf("got %v", err)
	}
}

func TestTagMemoryAndMaintain(t *testing.T) {
	s, c := newSvc(t)
	ctx := context.Background()
	m := save(t, s, "needs classification", nil)
	if err := s.TagMemory(ctx, m.ID, []string{"Project/Problems"}, store.SourceAgent); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Lattice.UntaggedCount(ctx); n != 0 {
		t.Error("tagging clears the queue")
	}
	if err := s.TagMemory(ctx, m.ID, []string{"  /!! "}, ""); err == nil {
		t.Error("empty paths rejected")
	}
	c.t = c.t.Add(400 * 24 * time.Hour)
	n, err := s.Maintain(ctx)
	if err != nil || n != 1 {
		t.Errorf("maintain dropped %d err %v", n, err)
	}
	ids, _ := s.Lattice.AllNodeIDs(ctx)
	found := false
	for _, id := range ids {
		found = found || id == "project/problems"
	}
	if !found {
		t.Error("maintenance must keep nodes")
	}
}

func TestActivityAndEvents(t *testing.T) {
	s, _ := newSvc(t)
	ch, cancel := s.Bus.Subscribe()
	defer cancel()
	s.RecordActivity(Activity{Tool: "recall", Source: "mcp"})
	if ev := <-ch; ev.Type != EvActivity {
		t.Errorf("event %+v", ev)
	}
	save(t, s, "x event", nil)
	seen := map[string]bool{}
	for len(ch) > 0 {
		seen[(<-ch).Type] = true
	}
	if !seen[EvMemory] {
		t.Errorf("save should publish a memory event: %v", seen)
	}
	if got := s.ActivityLog(); len(got) != 1 || got[0].ID != 1 {
		t.Errorf("log %+v", got)
	}
}

func TestAgo(t *testing.T) {
	cases := map[time.Duration]string{
		10 * time.Second: "just now", 5 * time.Minute: "5m ago", 3 * time.Hour: "3h ago",
		3 * 24 * time.Hour: "3d ago", 28 * 24 * time.Hour: "4w ago", 200 * 24 * time.Hour: "6mo ago",
	}
	for d, want := range cases {
		if got := Ago(d); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
}

func TestRetiredMemoriesLeaveProbeAndQueue(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	old := save(t, s, "blue green deploys", func(r *store.MemoryRecord) { r.LatticePaths = []string{"project/deploys"} })
	cur := save(t, s, "canary deploys", func(r *store.MemoryRecord) { r.LatticePaths = []string{"project/deploys"} })
	_, hits, _ := s.Probe(ctx, "deploys", 10)
	if len(hits) != 2 {
		t.Fatalf("both live: %v", hits)
	}
	s.Supersede(ctx, old.ID, cur.ID)
	if _, hits, _ = s.Probe(ctx, "deploys", 10); len(hits) != 1 || hits[0].ItemID != cur.ID {
		t.Errorf("superseded memory must not surface in a probe: %v", hits)
	}
	un := save(t, s, "unclassified then archived", nil)
	un2 := save(t, s, "unclassified then superseded", nil)
	if n, _ := s.Lattice.UntaggedCount(ctx); n != 2 {
		t.Fatalf("queue %d", n)
	}
	s.Archive(ctx, un.ID)
	s.Supersede(ctx, un2.ID, cur.ID)
	if n, _ := s.Lattice.UntaggedCount(ctx); n != 0 {
		t.Errorf("retired memories should leave the classification queue, left %d", n)
	}
}

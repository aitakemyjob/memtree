package store

import (
	"testing"
	"time"

	"github.com/aitakemyjob/memtree/internal/trace"
)

func traceEvent(id string, at time.Time, origin, session, project string, hits ...string) trace.Event {
	e := trace.Event{ID: id, At: at, Origin: origin, Tool: "recall", SessionID: session, Project: project, Query: "q " + id,
		Matches: []trace.Match{{Token: "redis", Category: "knowledge/tools/redis"}}}
	for i, m := range hits {
		e.Hits = append(e.Hits, trace.Hit{MemoryID: m, Rank: i + 1, Route: trace.RouteBoth, Via: "knowledge/tools/redis", Seed: "knowledge/tools", Depth: 1,
			Factors: trace.Factors{Text: 0.8, Link: 0.5, Importance: 1.5, Activation: 0.9, ProjectBoost: 1.25, BothBonus: 1.15, Final: 1.55}})
	}
	return e
}

func TestTracesRoundTripAndFilters(t *testing.T) {
	f := newFixture(t)
	tr := NewTraces(f.db)
	// Sub-second times would sort wrongly as RFC3339Nano text; fixed width keeps them ordered.
	at := t0.Add(500 * time.Millisecond)
	must(t, tr.Record(f.ctx, traceEvent("e1", t0, trace.OriginMCP, "s1", "/p", "mem_a", "mem_b")))
	must(t, tr.Record(f.ctx, traceEvent("e2", at, trace.OriginHook, "s2", "/p", "mem_a")))
	must(t, tr.Record(f.ctx, traceEvent("e3", t0.Add(time.Second), trace.OriginUI, "", "/q")))

	all, err := tr.List(f.ctx, trace.Filter{})
	if err != nil || len(all) != 3 || all[0].ID != "e3" || all[1].ID != "e2" || all[2].ID != "e1" {
		t.Fatalf("newest first: %+v %v", ids(all), err)
	}
	e1 := all[2]
	if len(e1.Hits) != 2 || e1.Hits[0].MemoryID != "mem_a" || e1.Hits[0].Factors.BothBonus != 1.15 || e1.Hits[0].Seed != "knowledge/tools" ||
		len(e1.Matches) != 1 || e1.Matches[0].Token != "redis" || !e1.At.Equal(t0) || e1.Uses == nil {
		t.Errorf("round trip: %+v", e1)
	}
	for name, c := range map[string]struct {
		f    trace.Filter
		want []string
	}{
		"origin":  {trace.Filter{Origins: []string{trace.OriginMCP, trace.OriginHook}}, []string{"e2", "e1"}},
		"session": {trace.Filter{SessionID: "s1"}, []string{"e1"}},
		"project": {trace.Filter{Project: "/q"}, []string{"e3"}},
		"memory":  {trace.Filter{MemoryID: "mem_b"}, []string{"e1"}},
		"quiet":   {trace.Filter{QuietOnly: true}, []string{"e3"}},
		"since":   {trace.Filter{Since: at}, []string{"e3", "e2"}},
		"until":   {trace.Filter{Until: at}, []string{"e2", "e1"}},
		"page":    {trace.Filter{Limit: 1, Offset: 1}, []string{"e2"}},
	} {
		got, err := tr.List(f.ctx, c.f)
		if err != nil || !equal(ids(got), c.want) {
			t.Errorf("%s: got %v want %v (%v)", name, ids(got), c.want, err)
		}
	}
	if _, err := tr.Get(f.ctx, "nope"); err == nil {
		t.Error("missing event should error")
	}
}

func TestTracesUseHeatDailyPrune(t *testing.T) {
	f := newFixture(t)
	tr := NewTraces(f.db)
	day2 := t0.Add(24 * time.Hour)
	must(t, tr.Record(f.ctx, traceEvent("e1", t0, trace.OriginMCP, "s", "/p", "mem_a", "mem_b")))
	must(t, tr.Record(f.ctx, traceEvent("e2", day2, trace.OriginMCP, "s", "/p", "mem_a")))
	use := trace.Use{EventID: "e2", MemoryID: "mem_a", Evidence: trace.EvidenceFetched, Detail: "memory_get", At: day2}
	must(t, tr.MarkUsed(f.ctx, []trace.Use{use, use, {EventID: "e2", MemoryID: "mem_a", Evidence: trace.EvidenceCited}}))
	e2, _ := tr.Get(f.ctx, "e2")
	if len(e2.Uses) != 2 {
		t.Errorf("repeated evidence is stored once per kind: %+v", e2.Uses)
	}

	heat, err := tr.Heat(f.ctx, trace.Filter{})
	if err != nil || len(heat) != 2 || heat[0].MemoryID != "mem_a" || heat[0].Retrieved != 2 || heat[0].Used != 1 || !heat[0].LastRetrieved.Equal(day2) {
		t.Errorf("heat: %+v %v", heat, err)
	}
	if h, _ := tr.Heat(f.ctx, trace.Filter{MemoryID: "mem_b"}); len(h) != 1 || h[0].Retrieved != 1 || h[0].Used != 0 {
		t.Errorf("heat for one memory: %+v", h)
	}
	daily, _ := tr.Daily(f.ctx, trace.Filter{})
	if len(daily) != 2 || daily[0].Day != "2026-06-22" || daily[0].Retrieved != 2 || daily[1].Used != 1 {
		t.Errorf("daily: %+v", daily)
	}
	if d, _ := tr.Daily(f.ctx, trace.Filter{MemoryID: "mem_a", Since: day2}); len(d) != 1 || d[0].Retrieved != 1 {
		t.Errorf("daily filtered: %+v", d)
	}

	n, err := tr.Prune(f.ctx, day2)
	if err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	left, _ := tr.List(f.ctx, trace.Filter{})
	if !equal(ids(left), []string{"e2"}) {
		t.Errorf("prune keeps newer events: %v", ids(left))
	}
	if h, _ := tr.Heat(f.ctx, trace.Filter{MemoryID: "mem_b"}); len(h) != 0 {
		t.Error("pruned hits must go with their event")
	}
}

func TestSignalMatchesAndActivationPath(t *testing.T) {
	f := newFixture(t)
	f.lat.SeedAxes(f.ctx)
	f.lat.EnsureNode(f.ctx, "knowledge/tools/sqlite")
	m, _ := f.lat.SignalMatches(f.ctx, "sqlite tools")
	want := map[trace.Match]bool{{Token: "sqlite", Category: "knowledge/tools/sqlite"}: true, {Token: "tools", Category: "knowledge/tools"}: true}
	for _, x := range m {
		delete(want, x)
	}
	if len(want) != 0 {
		t.Errorf("matches %v missing %v", m, want)
	}
	if c := MatchedCategories(m); !contains(c, "knowledge/tools") || !contains(c, "knowledge/tools/sqlite") {
		t.Errorf("categories %v", c)
	}

	f.lat.Link(f.ctx, "memory", "m", "knowledge/tools/sqlite", 0.9, "", "")
	hits, _ := f.lat.Activate(f.ctx, []string{"knowledge"}, ActivateOptions{Threshold: 0, Max: 5})
	if len(hits) != 1 || hits[0].Seed != "knowledge" || hits[0].Depth != 2 || hits[0].ViaCategory != "knowledge/tools/sqlite" {
		t.Errorf("activation path: %+v", hits)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func ids(es []trace.Event) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.ID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

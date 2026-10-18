package core

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/store"
	"github.com/aitakemyjob/memtree/internal/trace"
)

// product multiplies a hit's factors the way the score is defined.
func product(f trace.Factors) float64 {
	return math.Max(f.Text, f.Link) * f.Importance * f.Activation * f.ProjectBoost * f.BothBonus
}

func onlyEvent(t *testing.T, s *Service, fl trace.Filter) trace.Event {
	t.Helper()
	es, err := s.Traces.List(context.Background(), fl)
	if err != nil || len(es) != 1 {
		t.Fatalf("want one event, got %d (%v)", len(es), err)
	}
	return es[0]
}

func TestRecallRecordsFaithfulTrace(t *testing.T) {
	s, _ := newSvc(t)
	ctx := trace.WithOrigin(context.Background(), trace.Origin{Kind: trace.OriginMCP, Tool: "recall", Harness: "claude-code", Session: "mcp-1"})
	save(t, s, "redis eviction policy is allkeys-lru", func(r *store.MemoryRecord) { r.LatticePaths = []string{"knowledge/tools/redis"} })
	save(t, s, "eviction storms happen nightly", nil)
	save(t, s, "cache keys expire after an hour", func(r *store.MemoryRecord) { r.LatticePaths = []string{"knowledge/tools/redis"} })

	res, err := s.Recall(ctx, RecallInput{Query: "redis eviction", Project: "/p", Reinforce: true})
	if err != nil || res.TraceID == "" || len(res.Matches) == 0 {
		t.Fatalf("recall should return its trace and matches: %+v %v", res, err)
	}
	e, err := s.Traces.Get(context.Background(), res.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if e.Origin != trace.OriginMCP || e.Harness != "claude-code" || e.SessionID != "mcp-1" || e.Project != "/p" || !e.Reinforced || e.Query != "redis eviction" {
		t.Errorf("event context: %+v", e)
	}
	if len(e.Hits) != len(res.Memories) {
		t.Fatalf("hits %d vs memories %d", len(e.Hits), len(res.Memories))
	}
	routes := map[string]bool{}
	for i, h := range e.Hits {
		routes[h.Route] = true
		if h.Rank != i+1 || h.MemoryID != res.Memories[i].Record.ID {
			t.Errorf("rank order: %+v", h)
		}
		if math.Abs(product(h.Factors)-h.Factors.Final) > 1e-9 {
			t.Errorf("%s factors do not multiply to final: %+v", h.Route, h.Factors)
		}
		if h.Route != trace.RouteText && (h.Via == "" || h.Seed == "") {
			t.Errorf("lattice hits carry their path: %+v", h)
		}
	}
	if !routes[trace.RouteBoth] || !routes[trace.RouteText] || !routes[trace.RouteLattice] {
		t.Errorf("want every route covered, got %v", routes)
	}
}

func TestSurfaceAnchorAndProbeAreTraced(t *testing.T) {
	s, _ := newSvc(t)
	ctx := trace.WithOrigin(context.Background(), trace.Origin{Kind: trace.OriginHook, Tool: "UserPromptSubmit", Session: "s1"})
	m := save(t, s, "websocket reconnect uses exponential backoff", func(r *store.MemoryRecord) { r.LatticePaths = []string{"project/architecture/realtime"} })

	sf, _ := s.SurfaceForPrompt(ctx, "why does the websocket reconnect lag?", "/p")
	e, err := s.Traces.Get(context.Background(), sf.TraceID)
	if err != nil || len(e.Hits) != 1 || e.Hits[0].MemoryID != m.ID || e.Reinforced || e.SessionID != "s1" {
		t.Fatalf("surface trace: %+v %v", e, err)
	}
	if h := e.Hits[0]; math.Abs(product(h.Factors)-h.Factors.Final) > 1e-9 {
		t.Errorf("surface factors: %+v", h.Factors)
	}
	quiet, _ := s.SurfaceForPrompt(ctx, "please rename the variable", "/p")
	if quiet.Text != "" || quiet.TraceID == "" {
		t.Errorf("a quiet prompt is still traced: %+v", quiet)
	}

	if _, err := s.AnchorFor(trace.WithOrigin(context.Background(), trace.Origin{Kind: trace.OriginHook, Tool: "SessionStart"}), "/p", "s1"); err != nil {
		t.Fatal(err)
	}
	var anchor trace.Event
	all, _ := s.Traces.List(context.Background(), trace.Filter{})
	for _, e := range all {
		if e.Tool == "SessionStart" {
			anchor = e
		}
	}
	if anchor.Tool != "SessionStart" || len(anchor.Hits) != 1 || anchor.Hits[0].Route != trace.RouteAnchor || anchor.Hits[0].Via != "latest" {
		t.Errorf("anchor trace: %+v", anchor)
	}

	s.Probe(context.Background(), "realtime", 5)
	probe := onlyEvent(t, s, trace.Filter{Origins: []string{trace.OriginInternal}})
	if len(probe.Hits) != 1 || probe.Hits[0].Route != trace.RouteLattice {
		t.Errorf("probe trace: %+v", probe)
	}
}

func TestTracingSwitchUsesAndRetention(t *testing.T) {
	s, c := newSvc(t)
	ctx := trace.WithOrigin(context.Background(), trace.Origin{Kind: trace.OriginMCP, Session: "m"})
	m := save(t, s, "redis eviction policy is allkeys-lru", nil)

	s.Config.Update(func(x *config.Settings) { x.TraceRetrievals = false })
	s.Recall(ctx, RecallInput{Query: "redis", Project: "/p"})
	if es, _ := s.Traces.List(ctx, trace.Filter{}); len(es) != 0 {
		t.Fatalf("tracing off records nothing: %d", len(es))
	}
	s.Config.Update(func(x *config.Settings) { x.TraceRetrievals = true })
	res, _ := s.Recall(ctx, RecallInput{Query: "redis", Project: "/p"})

	s.Observe(ctx, trace.Observation{Kind: trace.ObservedToolCall, Tool: "memory_get", IDs: []string{m.ID}, Session: "m"})
	if e, _ := s.Traces.Get(ctx, res.TraceID); len(e.Uses) != 1 || e.Uses[0].Evidence != trace.EvidenceFetched {
		t.Errorf("fetch evidence: %+v", e.Uses)
	}

	c.t = c.t.Add(91 * 24 * time.Hour)
	if _, err := s.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if es, _ := s.Traces.List(ctx, trace.Filter{}); len(es) != 0 {
		t.Errorf("maintenance prunes traces past retention: %d left", len(es))
	}
}

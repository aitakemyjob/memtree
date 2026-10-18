package core

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aitakemyjob/memtree/internal/store"
	"github.com/aitakemyjob/memtree/internal/trace"
)

// useWindow is how long after a retrieval later activity can count as using it.
const useWindow = 30 * time.Minute

// wireTraces builds the trace pipeline over the store: recording is gated
// by the setting and announced on the bus; use evidence comes from the
// default detectors; retention follows the configured days.
func (s *Service) wireTraces(st *store.Traces) {
	enabled := func() bool { return s.Config.Get().TraceRetrievals }
	s.Trace = trace.Gated(enabled, trace.Publishing(func(e trace.Event) {
		s.emit(EvTrace, map[string]any{"op": "record", "id": e.ID})
	}, st))
	s.Traces = st
	s.Uses = trace.NewUseTracker(st, st, useWindow, s.DB.Now, trace.DefaultDetectors()...)
	s.Retention = trace.Retention{Pruner: st, Days: func() int { return s.Config.Get().TraceRetentionDays }, Now: s.DB.Now}
}

// recordRetrieval stores one retrieval attributed to the origin on ctx and
// returns its id. Tracing is best effort: a failure is reported on stderr
// and never fails the retrieval itself.
func (s *Service) recordRetrieval(ctx context.Context, project, query string, matches []trace.Match, hits []trace.Hit, reinforced bool) string {
	o := trace.OriginFrom(ctx)
	e := trace.Event{
		ID: store.NewID("ret"), At: s.DB.Now(), Origin: o.Kind, Tool: o.Tool, Harness: o.Harness, SessionID: o.Session,
		Project: project, Query: query, Matches: matches, Reinforced: reinforced, Hits: hits,
	}
	if err := s.Trace.Record(ctx, e); err != nil {
		fmt.Fprintf(os.Stderr, "memtree: trace not recorded: %v\n", err)
		return ""
	}
	return e.ID
}

// Observe feeds later agent activity to the use tracker. Like recording, it
// is best effort and silent when tracing is off.
func (s *Service) Observe(ctx context.Context, o trace.Observation) {
	if !s.Config.Get().TraceRetrievals {
		return
	}
	uses, err := s.Uses.Observe(ctx, o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memtree: trace use not recorded: %v\n", err)
		return
	}
	for _, u := range uses {
		s.emit(EvTrace, map[string]any{"op": "use", "id": u.EventID, "memoryId": u.MemoryID})
	}
}

// PruneTraces applies the retention setting once.
func (s *Service) PruneTraces(ctx context.Context) error {
	n, err := s.Retention.Run(ctx)
	if err == nil && n > 0 {
		s.emit(EvTrace, map[string]any{"op": "prune", "dropped": n})
	}
	return err
}

// factorsOf converts a score decomposition to trace factors, filling the
// neutral multipliers a route left unset so the product reads correctly.
func factorsOf(d store.ScoreDecomposition) trace.Factors {
	f := trace.Factors{
		Text: d.BM25Normalized, Link: d.LinkScore, Importance: d.ImportanceOffset, Activation: d.Activation,
		ProjectBoost: d.ProjectBoost, BothBonus: d.BothBonus, Final: d.Final,
	}
	if f.ProjectBoost == 0 {
		f.ProjectBoost = 1
	}
	if f.BothBonus == 0 {
		f.BothBonus = 1
	}
	return f
}

// hitsFromRecall maps a ranked recall result to trace hits.
func hitsFromRecall(rs []Recalled) []trace.Hit {
	out := make([]trace.Hit, len(rs))
	for i, r := range rs {
		out[i] = trace.Hit{MemoryID: r.Record.ID, Rank: i + 1, Route: r.Source, Via: r.Via, Seed: r.Seed, Depth: r.Depth, Factors: factorsOf(r.Score)}
	}
	return out
}

// hitsFromSurface maps the notes a hook injected to trace hits.
func hitsFromSurface(items []SurfaceItem) []trace.Hit {
	out := make([]trace.Hit, len(items))
	for i, it := range items {
		out[i] = trace.Hit{MemoryID: it.ID, Rank: i + 1, Route: it.Route, Via: it.Via, Seed: it.Seed, Depth: it.Depth, Factors: factorsOf(it.Factors)}
	}
	return out
}

// hitsFromProbe maps a lattice dry run to trace hits.
func hitsFromProbe(hits []store.ActivationHit) []trace.Hit {
	out := make([]trace.Hit, len(hits))
	for i, h := range hits {
		out[i] = trace.Hit{MemoryID: h.ItemID, Rank: i + 1, Route: trace.RouteLattice, Via: h.ViaCategory, Seed: h.Seed, Depth: h.Depth,
			Factors: trace.Factors{Link: h.Score, Importance: 1, Activation: 1, ProjectBoost: 1, BothBonus: 1, Final: h.Score}}
	}
	return out
}

// anchorHit is one memory a session anchor injected, and why.
type anchorHit struct {
	rec    store.MemoryRecord
	reason string // latest | landmark | pinned
}

// hitsFromAnchor maps session-start injections to trace hits.
func hitsFromAnchor(as []anchorHit) []trace.Hit {
	out := make([]trace.Hit, len(as))
	for i, a := range as {
		out[i] = trace.Hit{MemoryID: a.rec.ID, Rank: i + 1, Route: trace.RouteAnchor, Via: a.reason,
			Factors: trace.Factors{Importance: a.rec.Importance, Activation: 1, ProjectBoost: 1, BothBonus: 1}}
	}
	return out
}

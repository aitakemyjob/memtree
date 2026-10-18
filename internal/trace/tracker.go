package trace

import (
	"context"
	"sort"
	"time"
)

// candidateLimit bounds how many recent events one observation is matched against.
const candidateLimit = 100

// UseTracker turns observations into use evidence. It finds the recent
// retrievals an observation could relate to and lets each Detector judge;
// it holds no evidence rules itself.
type UseTracker struct {
	reader    Reader
	writer    UseWriter
	detectors []Detector
	window    time.Duration
	now       func() time.Time
}

// NewUseTracker returns a tracker matching observations to retrievals from
// the last window. Only agent-facing retrievals (MCP and hooks) count: a
// probe in the UI says nothing about what an agent relied on.
func NewUseTracker(r Reader, w UseWriter, window time.Duration, now func() time.Time, detectors ...Detector) *UseTracker {
	return &UseTracker{reader: r, writer: w, detectors: detectors, window: window, now: now}
}

// Observe records whatever use the observation evidences and returns it.
func (t *UseTracker) Observe(ctx context.Context, o Observation) ([]Use, error) {
	if o.At.IsZero() {
		o.At = t.now()
	}
	// Memory ids are unique, so any recent agent-facing retrieval that
	// returned the id is a candidate; affinity only decides which one is
	// credited. Hook and MCP sessions have different ids, which is why a
	// session match alone would miss a hook-surfaced note the agent fetched.
	events, err := t.reader.List(ctx, Filter{Origins: []string{OriginMCP, OriginHook}, Since: o.At.Add(-t.window), Until: o.At, Limit: candidateLimit})
	if err != nil {
		return nil, err
	}
	byAffinity(events, o)
	var uses []Use
	for _, d := range t.detectors {
		uses = append(uses, d.Detect(o, events)...)
	}
	if len(uses) == 0 {
		return nil, nil
	}
	return uses, t.writer.MarkUsed(ctx, uses)
}

// byAffinity orders candidates: same session first, then same project,
// keeping newest-first order within each group.
func byAffinity(events []Event, o Observation) {
	rank := func(e Event) int {
		switch {
		case o.Session != "" && e.SessionID == o.Session:
			return 0
		case o.Project != "" && e.Project == o.Project:
			return 1
		}
		return 2
	}
	sort.SliceStable(events, func(i, j int) bool { return rank(events[i]) < rank(events[j]) })
}

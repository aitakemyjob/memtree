package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aitakemyjob/memtree/internal/store"
	"github.com/aitakemyjob/memtree/internal/trace"
)

// RecallInput is a deliberate, agent- or human-initiated memory search.
type RecallInput struct {
	Query             string
	Project           string
	AllProjects       bool
	Limit             int
	Before, After     *time.Time
	Tier, Kind        string
	IncludeSuperseded bool
	Reinforce         bool // the caller will rely on what it gets back
}

// Recalled is one memory in a recall result, with why it matched.
type Recalled struct {
	Record store.MemoryRecord       `json:"record"`
	Score  store.ScoreDecomposition `json:"score"`
	Source string                   `json:"source"`         // text | lattice | both
	Via    string                   `json:"via,omitempty"`  // lattice category that surfaced it
	Seed   string                   `json:"seed,omitempty"` // fired category the lattice path started at
	Depth  int                      `json:"depth,omitempty"`
}

// RecallResult is the merged outcome of text search and lattice activation.
type RecallResult struct {
	Memories   []Recalled    `json:"memories"`
	Categories []string      `json:"categories"` // lattice nodes the query fired
	Matches    []trace.Match `json:"matches"`    // which query token fired which node
	TraceID    string        `json:"traceId,omitempty"`
}

// bothBonus lifts a memory found by both text and lattice: two independent
// signals agreeing is stronger evidence than either alone.
const bothBonus = 1.15

// Recall searches by text and by lattice association and merges the two.
// Text uses BM25 × importance × activation; the lattice fires the nodes the
// query matches and spreads activation to the memories linked there, scored
// link weight × node activation, then × importance × activation so both
// routes land on one comparable scale.
func (s *Service) Recall(ctx context.Context, in RecallInput) (RecallResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	q := store.Query{
		Text: in.Query, Limit: limit * 3, Project: in.Project, AllProjects: in.AllProjects,
		Before: in.Before, After: in.After, Tier: in.Tier, Kind: in.Kind, IncludeSuperseded: in.IncludeSuperseded,
	}
	text, err := s.Memory.Search(ctx, q)
	if err != nil {
		return RecallResult{}, err
	}
	matches, err := s.Lattice.SignalMatches(ctx, in.Query)
	if err != nil {
		return RecallResult{}, err
	}
	seeds := store.MatchedCategories(matches)
	merged := map[string]*Recalled{}
	var order []string
	for _, h := range text {
		h := h
		merged[h.Record.ID] = &Recalled{Record: h.Record, Score: h.Score, Source: "text"}
		order = append(order, h.Record.ID)
	}
	if len(seeds) > 0 {
		hits, err := s.Lattice.Activate(ctx, seeds, store.ActivateOptions{Threshold: 0.01, Max: limit * 3, ItemTypes: []string{store.ItemMemory}})
		if err != nil {
			return RecallResult{}, err
		}
		for _, h := range hits {
			r, err := s.Memory.Get(ctx, h.ItemID)
			if err != nil || !visible(r, q) {
				continue
			}
			act := s.Memory.ActivationOf(r)
			boost := 1.0
			if in.Project != "" && r.Project == in.Project {
				boost = store.ProjectBoost
			}
			final := h.Score * r.Importance * act * boost
			if cur, ok := merged[r.ID]; ok {
				cur.Source, cur.Via, cur.Seed, cur.Depth = "both", h.ViaCategory, h.Seed, h.Depth
				cur.Score.LinkScore, cur.Score.BothBonus = h.Score, bothBonus
				cur.Score.Final = max(cur.Score.Final, final) * bothBonus
				continue
			}
			merged[r.ID] = &Recalled{
				Record: r, Source: "lattice", Via: h.ViaCategory, Seed: h.Seed, Depth: h.Depth,
				Score: store.ScoreDecomposition{LinkScore: h.Score, ImportanceOffset: r.Importance, Activation: act, ProjectBoost: boost, Final: final},
			}
			order = append(order, r.ID)
		}
	}
	out := make([]Recalled, 0, len(order))
	for _, id := range order {
		out = append(out, *merged[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score.Final > out[j].Score.Final })
	if len(out) > limit {
		out = out[:limit]
	}
	if in.Reinforce && len(out) > 0 {
		ids := make([]string, len(out))
		for i, r := range out {
			ids[i] = r.Record.ID
		}
		if err := s.Memory.Touch(ctx, ids); err != nil {
			return RecallResult{}, err
		}
	}
	id := s.recordRetrieval(ctx, in.Project, in.Query, matches, hitsFromRecall(out), in.Reinforce && len(out) > 0)
	return RecallResult{Memories: out, Categories: nonNil(seeds), Matches: matches, TraceID: id}, nil
}

// visible applies a query's scope and lifecycle filters to a record fetched
// by id (the lattice route bypasses the SQL filters Search uses).
func visible(r store.MemoryRecord, q store.Query) bool {
	if !q.IncludeSuperseded && r.SupersededBy != "" {
		return false
	}
	if !q.IncludeArchived && r.ArchivedAt != nil {
		return false
	}
	if !store.InScope(r.Project, q.Project, q.AllProjects) {
		return false
	}
	if q.Tier != "" && r.Tier != q.Tier || q.Kind != "" && r.Kind != q.Kind {
		return false
	}
	if q.Before != nil && !r.CreatedAt.Before(*q.Before) || q.After != nil && r.CreatedAt.Before(*q.After) {
		return false
	}
	return true
}

// SurfaceItem is one note a hook injects.
type SurfaceItem struct {
	ID      string                   `json:"id"`
	Summary string                   `json:"summary"`
	Why     string                   `json:"why"`
	Score   float64                  `json:"score"`
	Route   string                   `json:"route"`          // text | lattice | both
	Via     string                   `json:"via,omitempty"`  // lattice category
	Seed    string                   `json:"seed,omitempty"` // fired category the path started at
	Depth   int                      `json:"depth,omitempty"`
	Factors store.ScoreDecomposition `json:"factors"` // the terms of the winning route's Score
}

// Surface is what a hook injects for a prompt.
type Surface struct {
	Items   []SurfaceItem `json:"items"`
	Text    string        `json:"text"` // ready to inject; "" means stay quiet
	TraceID string        `json:"traceId,omitempty"`
}

// surfaceTextCap limits text-route hits so the lattice route keeps room.
const surfaceTextCap = 2

// SurfaceForPrompt picks the few memories genuinely related to a prompt. It
// is deliberately conservative: content words must match, lattice hits must
// clear the configured threshold, the total is capped, and when nothing
// qualifies it returns nothing rather than something vague. It does not
// reinforce: only memories actually relied on are touched. No model is used.
func (s *Service) SurfaceForPrompt(ctx context.Context, prompt, project string) (Surface, error) {
	set := s.Config.Get()
	tokens := store.Keywords(prompt)
	if len(tokens) == 0 {
		return Surface{Items: []SurfaceItem{}}, nil
	}
	q := store.Query{Project: project}
	merged := map[string]SurfaceItem{}
	var order []string
	add := func(it SurfaceItem) {
		if cur, ok := merged[it.ID]; ok {
			if it.Score > cur.Score {
				cur.Score, cur.Factors = it.Score, it.Factors
			}
			if it.Via != "" {
				cur.Via, cur.Seed, cur.Depth = it.Via, it.Seed, it.Depth
			}
			cur.Route = trace.RouteBoth
			merged[it.ID] = cur
			return
		}
		merged[it.ID] = it
		order = append(order, it.ID)
	}
	text, err := s.Memory.Search(ctx, store.Query{Text: strings.Join(tokens, " "), Limit: surfaceTextCap, Project: project})
	if err != nil {
		return Surface{}, err
	}
	for _, h := range text {
		if h.Record.Kind == store.KindCapture {
			continue // raw captures are noise until summarised
		}
		add(SurfaceItem{ID: h.Record.ID, Summary: describe(h.Record, s.DB.Now()), Why: "matches your wording", Score: h.Score.Final,
			Route: trace.RouteText, Factors: h.Score})
	}
	matches, err := s.Lattice.SignalMatches(ctx, prompt)
	if err != nil {
		return Surface{}, err
	}
	seeds := store.MatchedCategories(matches)
	if len(seeds) > 0 {
		hits, err := s.Lattice.Activate(ctx, seeds, store.ActivateOptions{Threshold: set.SurfaceThreshold, Max: set.MaxSurface, ItemTypes: []string{store.ItemMemory}})
		if err != nil {
			return Surface{}, err
		}
		for _, h := range hits {
			r, err := s.Memory.Get(ctx, h.ItemID)
			if err != nil || !visible(r, q) || r.Kind == store.KindCapture {
				continue
			}
			// Surfacing scores the lattice route on link strength and
			// importance only; the factors say so rather than borrowing recall's.
			score := h.Score * r.Importance
			add(SurfaceItem{ID: r.ID, Summary: describe(r, s.DB.Now()), Why: "filed under " + h.ViaCategory, Score: score,
				Route: trace.RouteLattice, Via: h.ViaCategory, Seed: h.Seed, Depth: h.Depth,
				Factors: store.ScoreDecomposition{LinkScore: h.Score, ImportanceOffset: r.Importance, Activation: 1, ProjectBoost: 1, Final: score}})
		}
	}
	items := make([]SurfaceItem, 0, len(order))
	for _, id := range order {
		items = append(items, merged[id])
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	if set.MaxSurface > 0 && len(items) > set.MaxSurface {
		items = items[:set.MaxSurface]
	}
	id := s.recordRetrieval(ctx, project, prompt, matches, hitsFromSurface(items), false)
	return Surface{Items: items, Text: renderSurface(items), TraceID: id}, nil
}

// renderSurface formats surfaced notes as factual, caveated context lines.
func renderSurface(items []SurfaceItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("memtree: notes from earlier work that relate to this prompt (they may be outdated; check them against the code before relying on them):\n")
	for _, it := range items {
		fmt.Fprintf(&b, "- %s (%s; id %s)\n", it.Summary, it.Why, it.ID)
	}
	return strings.TrimRight(b.String(), "\n")
}

// describe renders a memory as a one-line, dated note.
func describe(r store.MemoryRecord, now time.Time) string {
	line := strings.Join(strings.Fields(r.Summary), " ")
	if len([]rune(line)) > 280 {
		line = string([]rune(line)[:280]) + "…"
	}
	return fmt.Sprintf("[%s] %s", Ago(now.Sub(r.CreatedAt)), line)
}

// Ago renders a duration as "just now", "5m ago", "3h ago", "2d ago", "4w ago".
func Ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 90*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	}
}

// nonNil keeps a slice from serialising as JSON null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// AnchorFor builds the session-start context (currentSession is excluded from the pending count): where work last left off in
// this project, one older landmark, pinned notes, open loops, and how many
// captured sessions still await a real summary. It is the port of cortex's
// recent-plus-strong "baseline floor" plus continuity. An empty store still
// returns a short pointer so the agent knows memory exists and how to use it.
func (s *Service) AnchorFor(ctx context.Context, project, currentSession string) (string, error) {
	q := store.Query{Project: project, Limit: 20}
	recent, err := s.Memory.List(ctx, q)
	if err != nil {
		return "", err
	}
	var live []store.MemoryRecord // newest first, excluding pinned (listed separately) and raw captures
	var pinned []store.MemoryRecord
	for _, r := range recent {
		switch {
		case r.Pinned:
			pinned = append(pinned, r)
		case r.Kind != store.KindCapture:
			live = append(live, r)
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return live[i].CreatedAt.After(live[j].CreatedAt) })
	now := s.DB.Now()
	var b strings.Builder
	var injected []anchorHit
	b.WriteString("memtree is connected: persistent memory for this project.\n")
	if len(live) > 0 {
		fmt.Fprintf(&b, "Last session notes — %s\n", describe(live[0], now))
		injected = append(injected, anchorHit{live[0], "latest"})
		for _, r := range live[1:] {
			if r.Importance >= store.ImportanceLesson {
				fmt.Fprintf(&b, "Landmark — %s\n", describe(r, now))
				injected = append(injected, anchorHit{r, "landmark"})
				break
			}
		}
	}
	for i, r := range pinned {
		if i == 3 {
			break
		}
		fmt.Fprintf(&b, "Pinned — %s\n", describe(r, now))
		injected = append(injected, anchorHit{r, "pinned"})
	}
	s.recordRetrieval(ctx, project, "", nil, hitsFromAnchor(injected), false)
	loops := openLoops(live, 5)
	if len(loops) > 0 {
		b.WriteString("Open loops:\n")
		for _, l := range loops {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	allPending, err := s.Capture.Pending(ctx, project, false, 0)
	if err != nil {
		return "", err
	}
	var pending []store.CaptureSession
	for _, p := range allPending {
		if p.ID != currentSession { // a resumed or compacted session is still in progress
			pending = append(pending, p)
		}
	}
	untagged, err := s.Lattice.UntaggedCount(ctx)
	if err != nil {
		return "", err
	}
	if len(pending) > 0 || untagged > 0 {
		b.WriteString("Memory upkeep, do this before the user's first task unless they need something urgent (it takes a few tool calls):\n")
		if len(pending) > 0 {
			fmt.Fprintf(&b, "- %d recent session(s) were captured but not summarised yet. Call `captures_pending`; for each, read it with `capture_transcript` and `memory_save` a 1-4 sentence summary with kind \"session\", its session_id, and lattice_paths. Skip sessions with nothing worth keeping by saving a one-line summary anyway, so they leave the queue.\n", len(pending))
		}
		if untagged > 0 {
			fmt.Fprintf(&b, "- %d memories have no confirmed lattice placement. Call `lattice_untagged`, then `lattice_tag` each (reuse categories from `lattice_browse`).\n", untagged)
		}
	}
	b.WriteString("Tools: `recall` to search memory, `memory_save` to keep decisions and lessons, `lattice_browse` to see how memory is organised.")
	return b.String(), nil
}

// openLoops collects up to max distinct open loops from records, newest first.
func openLoops(recs []store.MemoryRecord, max int) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range recs {
		if len(out) >= max {
			break
		}
		for _, l := range r.OpenLoops {
			if !seen[l] && len(out) < max {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out
}

// Probe shows what a piece of text lights up in the lattice: the fired
// categories and the live memories that would surface, with scores. Retired
// (superseded or archived) memories never appear, matching recall.
func (s *Service) Probe(ctx context.Context, text string, max int) ([]string, []store.ActivationHit, error) {
	matches, err := s.Lattice.SignalMatches(ctx, text)
	if err != nil {
		return nil, nil, err
	}
	seeds := store.MatchedCategories(matches)
	if len(seeds) == 0 {
		s.recordRetrieval(ctx, "", text, matches, nil, false)
		return seeds, []store.ActivationHit{}, nil
	}
	hits, err := s.Lattice.Activate(ctx, seeds, store.ActivateOptions{Threshold: 0.01, Max: max * 3, ItemTypes: []string{store.ItemMemory}})
	if err != nil {
		return nil, nil, err
	}
	live := []store.ActivationHit{}
	for _, h := range hits {
		r, err := s.Memory.Get(ctx, h.ItemID)
		if err != nil || r.SupersededBy != "" || r.ArchivedAt != nil {
			continue
		}
		live = append(live, h)
		if len(live) == max {
			break
		}
	}
	s.recordRetrieval(ctx, "", text, matches, hitsFromProbe(live), false)
	return seeds, live, nil
}

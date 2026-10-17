package store

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// AxisSeeds are the top-level shape of the lattice. They are created at
// startup so the firing path has nodes to match before anything is tagged.
// Agents grow children beneath them (e.g. "project/memtree", "person/brian").
var AxisSeeds = []string{
	"self", "self/identity", "self/preferences", "self/reflection",
	"project", "project/architecture", "project/decisions", "project/problems",
	"knowledge", "knowledge/domain", "knowledge/tools",
	"person", "action", "action/todo", "action/goal",
	"domain", "world", "time",
}

// AxisRoots are the seeds without a parent: the roots an agent should
// start paths from.
func AxisRoots() []string {
	var out []string
	for _, a := range AxisSeeds {
		if !strings.Contains(a, "/") {
			out = append(out, a)
		}
	}
	return out
}

// SeedAxes creates the axis nodes (idempotent).
func (l *Lattice) SeedAxes(ctx context.Context) error {
	for _, p := range AxisSeeds {
		if _, err := l.EnsureNode(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

var stopwords = toSet(strings.Fields(`
the a an and or but of to in on for with is are was were be been being do does did what whats how why when where
who which can could would should will i you me my your it its this that these those tell about know explain give show
help please some any get got latest current new more much many again really just thing things stuff work works details
detail from by as at into their there here we us our he she they them good all most very make made`))

// toSet builds a lookup set.
func toSet(in []string) map[string]bool {
	m := make(map[string]bool, len(in))
	for _, s := range in {
		m[s] = true
	}
	return m
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9\s]`)

// Keywords returns the significant tokens of text: lowercased, stopwords and
// tokens under 3 characters removed, de-duplicated, in first-seen order.
func Keywords(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(nonAlnum.ReplaceAllString(strings.ToLower(text), " ")) {
		if len(w) < 3 || stopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// SignalCategories turns text into the lattice nodes it fires: tokens are
// matched to existing node labels by equality or a ≥4-character containment
// in either direction. No content or no match fires nothing, which keeps an
// off-topic prompt quiet. The result is sorted for determinism.
func (l *Lattice) SignalCategories(ctx context.Context, text string) ([]string, error) {
	tokens := Keywords(text)
	if len(tokens) == 0 {
		return []string{}, nil
	}
	rows, err := l.db.sql.QueryContext(ctx, `SELECT id, label FROM category_nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := map[string]bool{}
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		for _, t := range tokens {
			if tokenMatchesLabel(t, label) {
				hits[id] = true
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(hits))
	for id := range hits {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// tokenMatchesLabel matches a token to a node label exactly or by a ≥4-character containment.
func tokenMatchesLabel(token, label string) bool {
	if token == label {
		return true
	}
	if len(token) >= 4 && strings.Contains(label, token) {
		return true
	}
	return len(label) >= 4 && strings.Contains(token, label)
}

// SuggestPaths proposes lattice paths for text by matching its keywords to
// existing nodes, deepest match first. It is deterministic and uses no model:
// it gives a raw capture a provisional placement so it can be recalled before
// an agent classifies it properly. Returns at most max paths.
func (l *Lattice) SuggestPaths(ctx context.Context, text string, max int) ([]string, error) {
	ids, err := l.SignalCategories(ctx, text)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(ids, func(i, j int) bool {
		di, dj := strings.Count(ids[i], "/"), strings.Count(ids[j], "/")
		if di != dj {
			return di > dj
		}
		return ids[i] < ids[j]
	})
	// A parent adds nothing when a matched child is already included.
	var out []string
	for _, id := range ids {
		covered := false
		for _, o := range out {
			if strings.HasPrefix(o, id+"/") {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, id)
		}
		if max > 0 && len(out) >= max {
			break
		}
	}
	return nonNil(out), nil
}

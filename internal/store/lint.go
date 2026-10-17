package store

import (
	"context"
	"sort"
	"strings"
)

// Lint finding limits keep reports readable.
const lintCap = 25

// NearDuplicate is a pair of sibling categories that probably mean the same thing.
type NearDuplicate struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Reason string `json:"reason"`
	LinksA int    `json:"linksA"`
	LinksB int    `json:"linksB"`
}

// SingleUse is a leaf category holding exactly one memory.
type SingleUse struct {
	Category string `json:"category"`
	MemoryID string `json:"memoryId"`
	Summary  string `json:"summary"`
}

// WeaklyFiled is a live memory with no agent- or human-confirmed placement:
// it has no links, or only keyword-guessed ("auto") ones.
type WeaklyFiled struct {
	MemoryID string   `json:"memoryId"`
	Summary  string   `json:"summary"`
	Paths    []string `json:"paths"`
}

// LintReport lists lattice hygiene problems. Every finding is advisory; none
// is fixed automatically.
type LintReport struct {
	NearDuplicates []NearDuplicate `json:"nearDuplicates"`
	OutsideAxes    []string        `json:"outsideAxes"` // nodes whose root is not a standard axis
	SingleUse      []SingleUse     `json:"singleUse"`
	WeaklyFiled    []WeaklyFiled   `json:"weaklyFiled"`
	Total          int             `json:"total"` // findings before the per-list cap
}

// PathWarning is advice about one path an agent is about to file under.
type PathWarning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Suggest string `json:"suggest,omitempty"` // an existing category to use instead
}

// similarLabels reports whether two sibling labels likely mean the same thing:
// equal after dropping separators and a plural s, one an abbreviation of the
// other ("db"/"database", "repo"/"repository"), or a typo apart.
func similarLabels(a, b string) (bool, string) {
	na, nb := squash(a), squash(b)
	if na == nb {
		return true, "same name apart from punctuation or plural"
	}
	short, long := na, nb
	if len(short) > len(long) {
		short, long = long, short
	}
	if abbreviates(short, long) {
		return true, "one is an abbreviation of the other"
	}
	ra, rb := strings.ToLower(a), strings.ToLower(b)
	if min(len(ra), len(rb)) >= 5 && editDistance(ra, rb) <= 2 {
		return true, "differ by a typo or inflection"
	}
	return false, ""
}

// abbreviates reports whether short (2–4 letters) abbreviates long: same
// first letter, its letters appear in order in long, and long is at least
// twice as long. "auth"/"author" is deliberately not a match.
func abbreviates(short, long string) bool {
	if len(short) < 2 || len(short) > 4 || len(long) < 2*len(short) || short[0] != long[0] {
		return false
	}
	i := 0
	for j := 0; j < len(long) && i < len(short); j++ {
		if long[j] == short[i] {
			i++
		}
	}
	return i == len(short)
}

// squash lowercases a label and removes separators and a trailing plural s.
func squash(s string) string {
	s = strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(s))
	if len(s) > 3 && strings.HasSuffix(s, "s") {
		s = s[:len(s)-1]
	}
	return s
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// Lint inspects the lattice and live memories for hygiene problems.
func (m *Memory) Lint(ctx context.Context) (LintReport, error) {
	rep := LintReport{NearDuplicates: []NearDuplicate{}, OutsideAxes: []string{}, SingleUse: []SingleUse{}, WeaklyFiled: []WeaklyFiled{}}
	nodes, err := m.lat.Nodes(ctx)
	if err != nil {
		return rep, err
	}
	seed := map[string]bool{}
	for _, s := range AxisSeeds {
		seed[s] = true
	}
	roots := map[string]bool{}
	for _, r := range AxisRoots() {
		roots[r] = true
	}
	byParent := map[string][]CategoryNode{}
	hasKids := map[string]bool{}
	for _, n := range nodes {
		byParent[n.ParentID] = append(byParent[n.ParentID], n)
		if n.ParentID != "" {
			hasKids[n.ParentID] = true
		}
	}
	parents := make([]string, 0, len(byParent))
	for p := range byParent {
		parents = append(parents, p)
	}
	sort.Strings(parents)
	for _, p := range parents {
		sib := byParent[p]
		sort.Slice(sib, func(i, j int) bool { return sib[i].ID < sib[j].ID })
		for i := 0; i < len(sib); i++ {
			for j := i + 1; j < len(sib); j++ {
				if ok, why := similarLabels(sib[i].Label, sib[j].Label); ok {
					rep.NearDuplicates = append(rep.NearDuplicates, NearDuplicate{sib[i].ID, sib[j].ID, why, sib[i].LinkCount, sib[j].LinkCount})
				}
			}
		}
	}
	for _, n := range nodes {
		root := strings.SplitN(n.ID, "/", 2)[0]
		if !roots[root] && n.ParentID == "" {
			rep.OutsideAxes = append(rep.OutsideAxes, n.ID)
		}
	}
	sort.Strings(rep.OutsideAxes)
	for _, n := range nodes {
		if seed[n.ID] || hasKids[n.ID] || n.LinkCount != 1 {
			continue
		}
		links, err := m.lat.LinksFor(ctx, n.ID)
		if err != nil || len(links) != 1 {
			continue
		}
		rep.SingleUse = append(rep.SingleUse, SingleUse{n.ID, links[0].ItemID, links[0].Summary})
	}
	sort.Slice(rep.SingleUse, func(i, j int) bool { return rep.SingleUse[i].Category < rep.SingleUse[j].Category })

	recs, err := m.List(ctx, Query{AllProjects: true})
	if err != nil {
		return rep, err
	}
	for _, r := range recs {
		if r.Kind == KindCapture {
			continue // raw captures are expected to be unfiled until summarised
		}
		links, err := m.lat.LinksForItem(ctx, ItemMemory, r.ID)
		if err != nil {
			return rep, err
		}
		confirmed := false
		for _, l := range links {
			if l.Source != SourceAuto {
				confirmed = true
			}
		}
		if !confirmed {
			rep.WeaklyFiled = append(rep.WeaklyFiled, WeaklyFiled{r.ID, truncate(r.Summary, 120), r.LatticePaths})
		}
	}
	rep.Total = len(rep.NearDuplicates) + len(rep.OutsideAxes) + len(rep.SingleUse) + len(rep.WeaklyFiled)
	rep.NearDuplicates = capList(rep.NearDuplicates)
	rep.OutsideAxes = capList(rep.OutsideAxes)
	rep.SingleUse = capList(rep.SingleUse)
	rep.WeaklyFiled = capList(rep.WeaklyFiled)
	return rep, nil
}

// capList truncates a finding list to lintCap entries.
func capList[T any](s []T) []T {
	if len(s) > lintCap {
		return s[:lintCap]
	}
	return s
}

// CheckPaths advises on paths about to be used: a root that is not a standard
// axis, or a new leaf that resembles an existing sibling. Advisory only; the
// caller still files under whatever it chose.
func (l *Lattice) CheckPaths(ctx context.Context, paths []string) ([]PathWarning, error) {
	nodes, err := l.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	children := map[string][]string{}
	for _, n := range nodes {
		exists[n.ID] = true
		children[n.ParentID] = append(children[n.ParentID], n.ID)
	}
	roots := map[string]bool{}
	for _, r := range AxisRoots() {
		roots[r] = true
	}
	out := []PathWarning{}
	for _, raw := range paths {
		p := NormalizePath(raw)
		if p == "" {
			continue
		}
		segs := strings.Split(p, "/")
		if !roots[segs[0]] && !exists[segs[0]] {
			out = append(out, PathWarning{Path: p, Message: "root \"" + segs[0] + "\" is not a standard axis (" + strings.Join(AxisRoots(), ", ") + "); filing here creates a new top-level category"})
			continue
		}
		cur := ""
		for _, seg := range segs {
			id := seg
			if cur != "" {
				id = cur + "/" + seg
			}
			if !exists[id] {
				for _, sib := range children[cur] {
					label := sib[strings.LastIndex(sib, "/")+1:]
					if ok, why := similarLabels(seg, label); ok {
						out = append(out, PathWarning{Path: p, Message: "new category \"" + id + "\" resembles existing \"" + sib + "\" (" + why + "); consider using it", Suggest: sib})
						break
					}
				}
				break // deeper segments are new under a new parent; nothing to compare
			}
			cur = id
		}
	}
	return out, nil
}

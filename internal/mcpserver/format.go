package mcpserver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aitakemyjob/memtree/internal/core"
	"github.com/aitakemyjob/memtree/internal/store"
)

// Agents get compact, line-oriented text rather than raw JSON: it is cheaper
// in tokens and reads naturally in a transcript.

// recordBlock renders a memory in a few lines.
func recordBlock(r store.MemoryRecord, now time.Time, extra string) string {
	var b strings.Builder
	scope := r.Project
	if scope == "" {
		scope = "global"
	}
	flags := []string{r.Kind, r.Tier, importanceName(r.Importance), core.Ago(now.Sub(r.CreatedAt))}
	if r.Pinned {
		flags = append(flags, "pinned")
	}
	if r.SupersededBy != "" {
		flags = append(flags, "superseded by "+r.SupersededBy)
	}
	if r.ArchivedAt != nil {
		flags = append(flags, "archived")
	}
	fmt.Fprintf(&b, "● %s [%s] %s\n", r.ID, strings.Join(flags, " · "), scope)
	if extra != "" {
		fmt.Fprintf(&b, "  %s\n", extra)
	}
	fmt.Fprintf(&b, "  %s\n", indent(r.Summary))
	if len(r.Decisions) > 0 {
		fmt.Fprintf(&b, "  decisions: %s\n", strings.Join(r.Decisions, "; "))
	}
	if len(r.OpenLoops) > 0 {
		fmt.Fprintf(&b, "  open loops: %s\n", strings.Join(r.OpenLoops, "; "))
	}
	if len(r.LatticePaths) > 0 {
		fmt.Fprintf(&b, "  lattice: %s\n", strings.Join(r.LatticePaths, ", "))
	}
	return b.String()
}

// indent indents continuation lines of a multi-line summary.
func indent(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n  ") }

// importanceName names an importance level: routine, decision or lesson.
func importanceName(v float64) string {
	switch {
	case v >= store.ImportanceLesson:
		return "lesson"
	case v >= store.ImportanceDecision:
		return "decision"
	}
	return "routine"
}

// orNone shows "(none)" for empty text.
func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

var relTime = regexp.MustCompile(`^(\d+)\s*(m|h|d|w|mo|y)$`)

// parseWhen accepts RFC 3339, a date (2026-07-01) or a relative age such as
// "30m", "24h", "7d", "2w", "3mo", "1y" meaning that long before now.
func parseWhen(s string, now time.Time) (*time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil, nil
	}
	if m := relTime.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		var d time.Duration
		switch m[2] {
		case "m":
			d = time.Duration(n) * time.Minute
		case "h":
			d = time.Duration(n) * time.Hour
		case "d":
			d = time.Duration(n) * 24 * time.Hour
		case "w":
			d = time.Duration(n) * 7 * 24 * time.Hour
		case "mo":
			d = time.Duration(n) * 30 * 24 * time.Hour
		case "y":
			d = time.Duration(n) * 365 * 24 * time.Hour
		}
		t := now.Add(-d)
		return &t, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("can't read time %q: use RFC 3339, YYYY-MM-DD, or a relative age like 7d, 24h, 2w", s)
}

// renderTree draws lattice nodes as an indented tree with link counts.
func renderTree(nodes []store.CategoryNode, root string, depth int) string {
	children := map[string][]store.CategoryNode{}
	var roots []store.CategoryNode
	for _, n := range nodes {
		if root != "" && n.ID == root {
			roots = append(roots, n)
			continue
		}
		if root == "" && n.ParentID == "" {
			roots = append(roots, n)
		}
		children[n.ParentID] = append(children[n.ParentID], n)
	}
	var b strings.Builder
	var walk func(n store.CategoryNode, lvl int)
	walk = func(n store.CategoryNode, lvl int) {
		total := subtreeLinks(n.ID, children) + n.LinkCount
		fmt.Fprintf(&b, "%s%s  (%d here, %d below)\n", strings.Repeat("  ", lvl), n.ID, n.LinkCount, total-n.LinkCount)
		if depth > 0 && lvl+1 >= depth {
			return
		}
		for _, c := range children[n.ID] {
			walk(c, lvl+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return strings.TrimRight(b.String(), "\n")
}

// subtreeLinks counts links in every descendant of a node.
func subtreeLinks(id string, children map[string][]store.CategoryNode) int {
	n := 0
	for _, c := range children[id] {
		n += c.LinkCount + subtreeLinks(c.ID, children)
	}
	return n
}

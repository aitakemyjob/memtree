package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Spreading-activation knobs (ported unchanged from cortex).
const (
	seedWeight    = 1.0
	distanceDecay = 0.5 // each hop to a parent/child multiplies activation by this
	maxDepth      = 2
	// DefaultLinkWeight is the weight of an agent- or human-placed link.
	DefaultLinkWeight = 0.6
	// AutoLinkWeight is the weight of a provisional link memtree placed itself
	// by keyword matching; it stays below the default surfacing threshold so
	// it helps recall without dominating until an agent confirms it.
	AutoLinkWeight = 0.35
	maxLinkSummary = 240
	maxUntagged    = 4000
)

// CategoryNode is one node of the lattice DAG. ID is the full slash path.
type CategoryNode struct {
	ID           string    `json:"id"`
	ParentID     string    `json:"parentId"` // "" for an axis root
	Label        string    `json:"label"`
	CreatedAt    time.Time `json:"createdAt"`
	AccessCount  int       `json:"accessCount"`
	LastAccessed time.Time `json:"lastAccessed"`
	LinkCount    int       `json:"linkCount"`
	Activation   float64   `json:"activation"` // current time-decay multiplier
}

// CategoryLink connects an item to a category with a decaying weight.
type CategoryLink struct {
	ItemType     string    `json:"itemType"`
	ItemID       string    `json:"itemId"`
	CategoryID   string    `json:"categoryId"`
	Weight       float64   `json:"weight"`
	Summary      string    `json:"summary"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"createdAt"`
	LastAccessed time.Time `json:"lastAccessed"`
}

// ActivationHit is an item surfaced by spreading activation.
type ActivationHit struct {
	ItemType    string  `json:"itemType"`
	ItemID      string  `json:"itemId"`
	Score       float64 `json:"score"`       // link weight × node activation
	ViaCategory string  `json:"viaCategory"` // strongest contributing node
	Summary     string  `json:"summary"`
}

// ActivateOptions bound an activation pass.
type ActivateOptions struct {
	Threshold float64
	Max       int
	ItemTypes []string // nil = all
	AxisRoots []string // nil = all
}

// UntaggedItem is an item waiting for an agent to classify it.
type UntaggedItem struct {
	ItemType string    `json:"itemType"`
	ItemID   string    `json:"itemId"`
	Text     string    `json:"text"`
	At       time.Time `json:"at"`
}

// Lattice is the category DAG plus spreading activation over it. Nodes are
// never deleted implicitly; links decay and faded ones are dropped.
type Lattice struct{ db *DB }

// NewLattice returns the lattice over db.
func NewLattice(db *DB) *Lattice { return &Lattice{db: db} }

var segmentJunk = regexp.MustCompile(`[^a-z0-9_-]`)

// NormalizePath lowercases a category path, strips punctuation from each
// segment and drops empty segments: "Self / Reflection!!" → "self/reflection".
func NormalizePath(path string) string {
	var segs []string
	for _, s := range strings.Split(strings.ToLower(path), "/") {
		s = segmentJunk.ReplaceAllString(strings.TrimSpace(s), "")
		if s != "" {
			segs = append(segs, s)
		}
	}
	return strings.Join(segs, "/")
}

// EnsureNode creates a path and its whole ancestor chain, returning the leaf id.
func (l *Lattice) EnsureNode(ctx context.Context, path string) (string, error) {
	var id string
	err := l.db.tx(ctx, func(t *sql.Tx) error {
		var err error
		id, err = l.ensureNodeQ(ctx, t, path)
		return err
	})
	return id, err
}

// ensureNodeQ is EnsureNode on an existing transaction.
func (l *Lattice) ensureNodeQ(ctx context.Context, q querier, path string) (string, error) {
	norm := NormalizePath(path)
	if norm == "" {
		return "", errors.New("empty category path")
	}
	at := ts(l.db.Now())
	parent := ""
	id := ""
	for _, label := range strings.Split(norm, "/") {
		if parent == "" {
			id = label
		} else {
			id = parent + "/" + label
		}
		var p any
		if parent != "" {
			p = parent
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO category_nodes (id, parent_id, label, created_at, access_count, last_accessed)
			 VALUES (?, ?, ?, ?, 0, ?) ON CONFLICT(id) DO NOTHING`, id, p, label, at, at); err != nil {
			return "", err
		}
		parent = id
	}
	return id, nil
}

// Link links an item to a category (creating the node chain), keeping the
// stronger weight if the link exists, and touches the node.
func (l *Lattice) Link(ctx context.Context, itemType, itemID, path string, weight float64, summary, source string) error {
	return l.db.tx(ctx, func(t *sql.Tx) error {
		return l.linkQ(ctx, t, itemType, itemID, path, weight, summary, source)
	})
}

// linkQ is Link on an existing transaction.
func (l *Lattice) linkQ(ctx context.Context, q querier, itemType, itemID, path string, weight float64, summary, source string) error {
	cat, err := l.ensureNodeQ(ctx, q, path)
	if err != nil {
		return err
	}
	if source == "" {
		source = SourceAgent
	}
	at := ts(l.db.Now())
	if _, err := q.ExecContext(ctx,
		`INSERT INTO category_links (item_type, item_id, category_id, weight, summary, source, created_at, last_accessed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(item_type, item_id, category_id) DO UPDATE SET
		   weight = max(weight, excluded.weight), summary = excluded.summary,
		   source = CASE WHEN excluded.source = 'auto' THEN source ELSE excluded.source END,
		   last_accessed = excluded.last_accessed`,
		itemType, itemID, cat, clamp01(weight), truncate(summary, maxLinkSummary), source, at, at); err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE category_nodes SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`, at, cat)
	return err
}

// Unlink removes one item→category link. The node stays.
func (l *Lattice) Unlink(ctx context.Context, itemType, itemID, path string) error {
	_, err := l.db.sql.ExecContext(ctx, `DELETE FROM category_links WHERE item_type = ? AND item_id = ? AND category_id = ?`,
		itemType, itemID, NormalizePath(path))
	return err
}

// unlinkAllQ removes every link an item has.
func (l *Lattice) unlinkAllQ(ctx context.Context, q querier, itemType, itemID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM category_links WHERE item_type = ? AND item_id = ?`, itemType, itemID)
	return err
}

// SetLinks replaces an item's links with paths (used when a human or agent
// re-classifies a record).
func (l *Lattice) SetLinks(ctx context.Context, itemType, itemID string, paths []string, summary, source string) error {
	return l.db.tx(ctx, func(t *sql.Tx) error {
		if err := l.unlinkAllQ(ctx, t, itemType, itemID); err != nil {
			return err
		}
		for _, p := range paths {
			if err := l.linkQ(ctx, t, itemType, itemID, p, DefaultLinkWeight, summary, source); err != nil {
				return err
			}
		}
		return l.clearUntaggedQ(ctx, t, itemType, itemID)
	})
}

// Reinforce strengthens every link an item has (it was used) and touches the nodes.
func (l *Lattice) Reinforce(ctx context.Context, itemType, itemID string, by float64) error {
	return l.db.tx(ctx, func(t *sql.Tx) error {
		at := ts(l.db.Now())
		cats, err := l.itemCategoriesQ(ctx, t, itemType, itemID)
		if err != nil {
			return err
		}
		for _, c := range cats {
			if _, err := t.ExecContext(ctx,
				`UPDATE category_links SET weight = min(1, weight + ?), last_accessed = ? WHERE item_type = ? AND item_id = ? AND category_id = ?`,
				by, at, itemType, itemID, c); err != nil {
				return err
			}
			if _, err := t.ExecContext(ctx, `UPDATE category_nodes SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`, at, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// DecayLinks ages every link by an exponential half-life from its last
// access and drops links that fall below minWeight. Returns links dropped.
func (l *Lattice) DecayLinks(ctx context.Context, halfLifeDays, minWeight float64) (int, error) {
	if halfLifeDays <= 0 {
		return 0, nil
	}
	now := l.db.Now()
	links, err := l.queryLinks(ctx, l.db.sql, `SELECT * FROM category_links`)
	if err != nil {
		return 0, err
	}
	dropped := 0
	err = l.db.tx(ctx, func(t *sql.Tx) error {
		for _, k := range links {
			age := math.Max(0, now.Sub(k.LastAccessed).Hours()/24)
			w := k.Weight * math.Pow(0.5, age/halfLifeDays)
			if w < minWeight {
				if _, err := t.ExecContext(ctx, `DELETE FROM category_links WHERE item_type = ? AND item_id = ? AND category_id = ?`, k.ItemType, k.ItemID, k.CategoryID); err != nil {
					return err
				}
				dropped++
			} else if w != k.Weight {
				// Ageing is measured from last access, so stamp it to keep
				// repeated passes from compounding the same interval.
				if _, err := t.ExecContext(ctx, `UPDATE category_links SET weight = ?, last_accessed = ? WHERE item_type = ? AND item_id = ? AND category_id = ?`, w, ts(now), k.ItemType, k.ItemID, k.CategoryID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return dropped, err
}

// Activate runs spreading activation: seeds start at full weight and spread
// to parents and children (distance-decayed, bounded depth); each node's
// activation is further scaled by its own time decay. Linked items score
// link weight × node activation, keeping their best path. Note the BFS
// propagates the undecayed activation to neighbours and stores the
// time-decayed value per node, matching cortex's behaviour.
func (l *Lattice) Activate(ctx context.Context, seeds []string, opts ActivateOptions) ([]ActivationHit, error) {
	now := l.db.Now()
	axisOK := func(id string) bool {
		if opts.AxisRoots == nil {
			return true
		}
		for _, r := range opts.AxisRoots {
			if id == r || strings.HasPrefix(id, r+"/") {
				return true
			}
		}
		return false
	}
	graph, err := l.loadGraph(ctx)
	if err != nil {
		return nil, err
	}
	type step struct {
		id    string
		act   float64
		depth int
	}
	var frontier []step
	for _, s := range seeds {
		if _, ok := graph.nodes[s]; ok && axisOK(s) {
			frontier = append(frontier, step{s, seedWeight, 0})
		}
	}
	if len(frontier) == 0 {
		return []ActivationHit{}, nil
	}
	activation := map[string]float64{}
	var order []string // deterministic iteration order
	for len(frontier) > 0 {
		var next []step
		for _, s := range frontier {
			decayed := s.act * ActivationScore([]time.Time{graph.nodes[s.id].LastAccessed}, NodeParams, now)
			prev, seen := activation[s.id]
			if decayed <= prev {
				continue
			}
			if !seen {
				order = append(order, s.id)
			}
			activation[s.id] = decayed
			if s.depth >= maxDepth {
				continue
			}
			for _, n := range graph.neighbors(s.id) {
				if axisOK(n) {
					next = append(next, step{n, s.act * distanceDecay, s.depth + 1})
				}
			}
		}
		frontier = next
	}
	typeOK := func(t string) bool {
		if opts.ItemTypes == nil {
			return true
		}
		for _, x := range opts.ItemTypes {
			if x == t {
				return true
			}
		}
		return false
	}
	best := map[string]ActivationHit{}
	var keys []string
	for _, cat := range order {
		links, err := l.queryLinks(ctx, l.db.sql, `SELECT * FROM category_links WHERE category_id = ?`, cat)
		if err != nil {
			return nil, err
		}
		for _, k := range links {
			if !typeOK(k.ItemType) {
				continue
			}
			score := k.Weight * activation[cat]
			key := k.ItemType + ":" + k.ItemID
			cur, ok := best[key]
			if !ok {
				keys = append(keys, key)
			}
			if !ok || score > cur.Score {
				best[key] = ActivationHit{ItemType: k.ItemType, ItemID: k.ItemID, Score: score, ViaCategory: cat, Summary: k.Summary}
			}
		}
	}
	hits := []ActivationHit{}
	for _, k := range keys {
		if h := best[k]; h.Score >= opts.Threshold {
			hits = append(hits, h)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if opts.Max > 0 && len(hits) > opts.Max {
		hits = hits[:opts.Max]
	}
	return hits, nil
}

// Nodes returns every node with its link count and current time decay,
// most-accessed first.
func (l *Lattice) Nodes(ctx context.Context) ([]CategoryNode, error) {
	rows, err := l.db.sql.QueryContext(ctx, `
		SELECT n.id, COALESCE(n.parent_id, ''), n.label, n.created_at, n.access_count, n.last_accessed,
		       (SELECT COUNT(*) FROM category_links k WHERE k.category_id = n.id)
		FROM category_nodes n ORDER BY n.access_count DESC, n.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := l.db.Now()
	out := []CategoryNode{}
	for rows.Next() {
		var n CategoryNode
		var created, last string
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Label, &created, &n.AccessCount, &last, &n.LinkCount); err != nil {
			return nil, err
		}
		n.CreatedAt, n.LastAccessed = parseTS(created), parseTS(last)
		n.Activation = ActivationScore([]time.Time{n.LastAccessed}, NodeParams, now)
		out = append(out, n)
	}
	return out, rows.Err()
}

// AllNodeIDs returns every node id, sorted.
func (l *Lattice) AllNodeIDs(ctx context.Context) ([]string, error) {
	rows, err := l.db.sql.QueryContext(ctx, `SELECT id FROM category_nodes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LinksFor returns the links into one category, strongest first.
func (l *Lattice) LinksFor(ctx context.Context, path string) ([]CategoryLink, error) {
	return l.queryLinks(ctx, l.db.sql, `SELECT * FROM category_links WHERE category_id = ? ORDER BY weight DESC`, NormalizePath(path))
}

// LinksForItem returns one item's links, strongest first.
func (l *Lattice) LinksForItem(ctx context.Context, itemType, itemID string) ([]CategoryLink, error) {
	return l.queryLinks(ctx, l.db.sql, `SELECT * FROM category_links WHERE item_type = ? AND item_id = ? ORDER BY weight DESC`, itemType, itemID)
}

// itemCategoriesQ lists the categories an item is linked to, strongest first.
func (l *Lattice) itemCategoriesQ(ctx context.Context, q querier, itemType, itemID string) ([]string, error) {
	links, err := l.queryLinks(ctx, q, `SELECT * FROM category_links WHERE item_type = ? AND item_id = ? ORDER BY weight DESC`, itemType, itemID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(links))
	for i, k := range links {
		out[i] = k.CategoryID
	}
	return out, nil
}

// MergeNode moves every link from one node onto another (creating it) and
// removes the source node if it is now empty and childless. Used to tidy a
// lattice that grew near-duplicate categories.
func (l *Lattice) MergeNode(ctx context.Context, from, to string) error {
	from, to = NormalizePath(from), NormalizePath(to)
	if from == "" || to == "" || from == to {
		return errors.New("merge needs two different category paths")
	}
	return l.db.tx(ctx, func(t *sql.Tx) error {
		links, err := l.queryLinks(ctx, t, `SELECT * FROM category_links WHERE category_id = ?`, from)
		if err != nil {
			return err
		}
		for _, k := range links {
			if err := l.linkQ(ctx, t, k.ItemType, k.ItemID, to, k.Weight, k.Summary, k.Source); err != nil {
				return err
			}
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM category_links WHERE category_id = ?`, from); err != nil {
			return err
		}
		_, err = t.ExecContext(ctx, `DELETE FROM category_nodes WHERE id = ? AND NOT EXISTS (SELECT 1 FROM category_nodes c WHERE c.parent_id = ?)`, from, from)
		return err
	})
}

// PruneEmpty deletes leaf nodes with no links, except the protected paths
// (the axis seeds). It repeats until stable so empty chains collapse.
// Returns how many nodes were removed.
func (l *Lattice) PruneEmpty(ctx context.Context, protected []string) (int, error) {
	keep := map[string]bool{}
	for _, p := range protected {
		keep[p] = true
	}
	total := 0
	for {
		rows, err := l.db.sql.QueryContext(ctx, `
			SELECT id FROM category_nodes n
			WHERE NOT EXISTS (SELECT 1 FROM category_links k WHERE k.category_id = n.id)
			  AND NOT EXISTS (SELECT 1 FROM category_nodes c WHERE c.parent_id = n.id)`)
		if err != nil {
			return total, err
		}
		var victims []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return total, err
			}
			if !keep[id] {
				victims = append(victims, id)
			}
		}
		rows.Close()
		if len(victims) == 0 {
			return total, nil
		}
		for _, id := range victims {
			if _, err := l.db.sql.ExecContext(ctx, `DELETE FROM category_nodes WHERE id = ?`, id); err != nil {
				return total, err
			}
		}
		total += len(victims)
	}
}

// EnqueueUntagged queues an item for classification. Empty text is dropped;
// re-enqueuing refreshes the text.
func (l *Lattice) EnqueueUntagged(ctx context.Context, itemType, itemID, text string) error {
	return l.enqueueQ(ctx, l.db.sql, itemType, itemID, text)
}

// enqueueQ is EnqueueUntagged on an existing transaction.
func (l *Lattice) enqueueQ(ctx context.Context, q querier, itemType, itemID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO lattice_untagged (item_type, item_id, text, at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(item_type, item_id) DO UPDATE SET text = excluded.text, at = excluded.at`,
		itemType, itemID, truncate(text, maxUntagged), ts(l.db.Now()))
	return err
}

// Untagged lists queued items oldest first (limit ≤ 0 = all).
func (l *Lattice) Untagged(ctx context.Context, limit int) ([]UntaggedItem, error) {
	if limit <= 0 {
		limit = -1
	}
	rows, err := l.db.sql.QueryContext(ctx, `SELECT item_type, item_id, text, at FROM lattice_untagged ORDER BY at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UntaggedItem{}
	for rows.Next() {
		var u UntaggedItem
		var at string
		if err := rows.Scan(&u.ItemType, &u.ItemID, &u.Text, &at); err != nil {
			return nil, err
		}
		u.At = parseTS(at)
		out = append(out, u)
	}
	return out, rows.Err()
}

// ClearUntagged removes an item from the queue.
func (l *Lattice) ClearUntagged(ctx context.Context, itemType, itemID string) error {
	return l.clearUntaggedQ(ctx, l.db.sql, itemType, itemID)
}

// clearUntaggedQ is ClearUntagged on an existing transaction.
func (l *Lattice) clearUntaggedQ(ctx context.Context, q querier, itemType, itemID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM lattice_untagged WHERE item_type = ? AND item_id = ?`, itemType, itemID)
	return err
}

// UntaggedCount is the queue length.
func (l *Lattice) UntaggedCount(ctx context.Context) (int, error) {
	var n int
	err := l.db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM lattice_untagged`).Scan(&n)
	return n, err
}

// graph is an in-memory snapshot of the node DAG for one activation pass.
type graph struct {
	nodes    map[string]CategoryNode
	children map[string][]string
}

// neighbors returns a node's parent and children, the edges activation spreads along.
func (g graph) neighbors(id string) []string {
	var out []string
	if p := g.nodes[id].ParentID; p != "" {
		out = append(out, p)
	}
	return append(out, g.children[id]...)
}

// loadGraph snapshots every node and its children for one activation pass.
func (l *Lattice) loadGraph(ctx context.Context) (graph, error) {
	g := graph{nodes: map[string]CategoryNode{}, children: map[string][]string{}}
	rows, err := l.db.sql.QueryContext(ctx, `SELECT id, COALESCE(parent_id, ''), label, last_accessed FROM category_nodes ORDER BY id`)
	if err != nil {
		return g, err
	}
	defer rows.Close()
	for rows.Next() {
		var n CategoryNode
		var last string
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Label, &last); err != nil {
			return g, err
		}
		n.LastAccessed = parseTS(last)
		g.nodes[n.ID] = n
		if n.ParentID != "" {
			g.children[n.ParentID] = append(g.children[n.ParentID], n.ID)
		}
	}
	return g, rows.Err()
}

// queryLinks runs a link query (written as SELECT *) and scans the rows.
func (l *Lattice) queryLinks(ctx context.Context, q querier, query string, args ...any) ([]CategoryLink, error) {
	rows, err := q.QueryContext(ctx, strings.Replace(query, "SELECT *",
		"SELECT item_type, item_id, category_id, weight, summary, source, created_at, last_accessed", 1), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CategoryLink{}
	for rows.Next() {
		var k CategoryLink
		var created, last string
		if err := rows.Scan(&k.ItemType, &k.ItemID, &k.CategoryID, &k.Weight, &k.Summary, &k.Source, &created, &last); err != nil {
			return nil, err
		}
		k.CreatedAt, k.LastAccessed = parseTS(created), parseTS(last)
		out = append(out, k)
	}
	return out, rows.Err()
}

// clamp01 clamps f into [0, 1].
func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

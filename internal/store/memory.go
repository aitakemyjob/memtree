package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("memory not found")

// Memory is the memory store: records, their FTS index, and scoring.
// It places new records on the lattice through the Lattice it wraps.
type Memory struct {
	db  *DB
	lat *Lattice
}

// NewMemory returns the memory store over db and lat.
func NewMemory(db *DB, lat *Lattice) *Memory { return &Memory{db: db, lat: lat} }

// NewID returns a sortable-ish unique id with a short prefix.
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// SnapImportance maps any positive number to the nearest of the three
// importance levels; non-positive means routine.
func SnapImportance(v float64) float64 {
	switch {
	case v >= 2.0:
		return ImportanceLesson
	case v >= 1.25:
		return ImportanceDecision
	default:
		return ImportanceRoutine
	}
}

// normalize fills defaults and cleans a record in place.
func (m *Memory) normalize(r *MemoryRecord) error {
	r.Summary = strings.TrimSpace(r.Summary)
	if r.Summary == "" {
		return errors.New("summary is required")
	}
	now := m.db.Now()
	if r.ID == "" {
		r.ID = NewID("mem")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	if r.Tier != TierSemantic {
		r.Tier = TierEpisodic
	}
	if r.Kind == "" {
		r.Kind = KindNote
	}
	r.Importance = SnapImportance(r.Importance)
	if r.Provenance == "" {
		r.Provenance = ProvInferred
	}
	if r.Harness == "" {
		r.Harness = "mcp"
	}
	r.Keywords = cleanStrings(r.Keywords)
	if len(r.Keywords) == 0 {
		r.Keywords = Keywords(r.Summary)
		if len(r.Keywords) > 12 {
			r.Keywords = r.Keywords[:12]
		}
	}
	r.Decisions = cleanStrings(r.Decisions)
	r.OpenLoops = cleanStrings(r.OpenLoops)
	r.LatticePaths = cleanStrings(r.LatticePaths)
	if len(r.AccessLog) == 0 {
		// Creation counts as the first access, so a new memory starts warm
		// and cools with age. (cortex started with an empty log, which left
		// every unused memory at the floor regardless of age.)
		r.AccessLog = []time.Time{r.CreatedAt}
	}
	return nil
}

// Save stores a record. LatticePaths are linked (agent-confirmed, default
// weight). Without paths the record is queued for classification and given a
// provisional keyword-matched placement so it is recallable immediately.
func (m *Memory) Save(ctx context.Context, r MemoryRecord) (MemoryRecord, error) {
	if err := m.normalize(&r); err != nil {
		return r, err
	}
	provisional := []string{}
	if len(r.LatticePaths) == 0 {
		p, err := m.lat.SuggestPaths(ctx, r.Summary+" "+strings.Join(r.Keywords, " "), 3)
		if err != nil {
			return r, err
		}
		provisional = p
	}
	err := m.db.tx(ctx, func(t *sql.Tx) error {
		if err := m.insertQ(ctx, t, r); err != nil {
			return err
		}
		for _, p := range r.LatticePaths {
			if err := m.lat.linkQ(ctx, t, ItemMemory, r.ID, p, DefaultLinkWeight, r.Summary, SourceAgent); err != nil {
				return err
			}
		}
		if len(r.LatticePaths) == 0 {
			for _, p := range provisional {
				if err := m.lat.linkQ(ctx, t, ItemMemory, r.ID, p, AutoLinkWeight, r.Summary, SourceAuto); err != nil {
					return err
				}
			}
			if r.Kind == KindCapture {
				return nil // raw captures get a summary, not a classification
			}
			return m.lat.enqueueQ(ctx, t, ItemMemory, r.ID, r.Summary)
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	if len(r.LatticePaths) == 0 {
		r.LatticePaths = provisional
	}
	return r, nil
}

// insertQ writes a new record and its full-text row.
func (m *Memory) insertQ(ctx context.Context, q querier, r MemoryRecord) error {
	blob, err := json.Marshal(r)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO memory_records (id, created_at, updated_at, project, harness, session_id, tier, kind, importance, pinned, superseded_by, archived_at, json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, ts(r.CreatedAt), ts(r.UpdatedAt), r.Project, r.Harness, r.SessionID, r.Tier, r.Kind, r.Importance,
		boolInt(r.Pinned), nullStr(r.SupersededBy), nullTS(r.ArchivedAt), string(blob))
	if err != nil {
		return err
	}
	rowid, err := res.LastInsertId()
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO memory_fts(rowid, summary, keywords, decisions) VALUES (?, ?, ?, ?)`,
		rowid, r.Summary, strings.Join(r.Keywords, " "), strings.Join(r.Decisions, " "))
	return err
}

// Update replaces a record's editable content. Bookkeeping (created time,
// access log, supersession) is preserved. If paths is non-nil the record's
// lattice links are replaced by it.
func (m *Memory) Update(ctx context.Context, id string, edit func(*MemoryRecord), paths []string) (MemoryRecord, error) {
	var out MemoryRecord
	err := m.db.tx(ctx, func(t *sql.Tx) error {
		cur, rowid, err := m.getQ(ctx, t, id)
		if err != nil {
			return err
		}
		next := cur
		edit(&next)
		next.ID, next.CreatedAt = cur.ID, cur.CreatedAt
		next.AccessLog, next.SupersededBy, next.ArchivedAt = cur.AccessLog, cur.SupersededBy, cur.ArchivedAt
		if err := m.normalize(&next); err != nil {
			return err
		}
		if err := m.writeBackQ(ctx, t, rowid, next); err != nil {
			return err
		}
		if _, err := t.ExecContext(ctx, `UPDATE memory_fts SET summary = ?, keywords = ?, decisions = ? WHERE rowid = ?`,
			next.Summary, strings.Join(next.Keywords, " "), strings.Join(next.Decisions, " "), rowid); err != nil {
			return err
		}
		if paths != nil {
			if err := m.lat.unlinkAllQ(ctx, t, ItemMemory, id); err != nil {
				return err
			}
			for _, p := range cleanStrings(paths) {
				if err := m.lat.linkQ(ctx, t, ItemMemory, id, p, DefaultLinkWeight, next.Summary, SourceHuman); err != nil {
					return err
				}
			}
			if err := m.lat.clearUntaggedQ(ctx, t, ItemMemory, id); err != nil {
				return err
			}
		}
		out = next
		return nil
	})
	if err != nil {
		return out, err
	}
	return m.fillPaths(ctx, out)
}

// writeBackQ persists a modified record's columns and JSON blob.
func (m *Memory) writeBackQ(ctx context.Context, q querier, rowid int64, r MemoryRecord) error {
	blob, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx,
		`UPDATE memory_records SET updated_at = ?, project = ?, harness = ?, session_id = ?, tier = ?, kind = ?, importance = ?,
		   pinned = ?, superseded_by = ?, archived_at = ?, json = ? WHERE rowid = ?`,
		ts(r.UpdatedAt), r.Project, r.Harness, r.SessionID, r.Tier, r.Kind, r.Importance,
		boolInt(r.Pinned), nullStr(r.SupersededBy), nullTS(r.ArchivedAt), string(blob), rowid)
	return err
}

// Get returns one record with its lattice paths.
func (m *Memory) Get(ctx context.Context, id string) (MemoryRecord, error) {
	r, _, err := m.getQ(ctx, m.db.sql, id)
	if err != nil {
		return r, err
	}
	return m.fillPaths(ctx, r)
}

// getQ loads a record and its rowid.
func (m *Memory) getQ(ctx context.Context, q querier, id string) (MemoryRecord, int64, error) {
	var blob string
	var rowid int64
	err := q.QueryRowContext(ctx, `SELECT rowid, json FROM memory_records WHERE id = ?`, id).Scan(&rowid, &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return MemoryRecord{}, 0, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return MemoryRecord{}, 0, err
	}
	r, err := decode(blob)
	return r, rowid, err
}

// decode unmarshals a stored record, normalising nil lists.
func decode(blob string) (MemoryRecord, error) {
	var r MemoryRecord
	if err := json.Unmarshal([]byte(blob), &r); err != nil {
		return r, err
	}
	r.Keywords, r.Decisions, r.OpenLoops = nonNil(r.Keywords), nonNil(r.Decisions), nonNil(r.OpenLoops)
	if r.AccessLog == nil {
		r.AccessLog = []time.Time{}
	}
	r.LatticePaths = nonNil(r.LatticePaths)
	return r, nil
}

// fillPaths sets LatticePaths from the live links.
func (m *Memory) fillPaths(ctx context.Context, r MemoryRecord) (MemoryRecord, error) {
	cats, err := m.lat.itemCategoriesQ(ctx, m.db.sql, ItemMemory, r.ID)
	if err != nil {
		return r, err
	}
	r.LatticePaths = nonNil(cats)
	return r, nil
}

// scopeSQL builds the WHERE clause shared by Search and List.
func scopeSQL(q Query) (string, []any) {
	var where []string
	var args []any
	if !q.IncludeSuperseded {
		where = append(where, "r.superseded_by IS NULL")
	}
	if !q.IncludeArchived {
		where = append(where, "r.archived_at IS NULL")
	}
	if !q.AllProjects {
		where = append(where, "(r.project = '' OR r.project = ?)")
		args = append(args, q.Project)
	}
	if q.Tier != "" {
		where = append(where, "r.tier = ?")
		args = append(args, q.Tier)
	}
	if q.Kind != "" {
		where = append(where, "r.kind = ?")
		args = append(args, q.Kind)
	}
	if q.ExcludeKind != "" {
		where = append(where, "r.kind != ?")
		args = append(args, q.ExcludeKind)
	}
	if q.PinnedOnly {
		where = append(where, "r.pinned = 1")
	}
	if q.Before != nil {
		where = append(where, "r.created_at < ?")
		args = append(args, ts(*q.Before))
	}
	if q.After != nil {
		where = append(where, "r.created_at >= ?")
		args = append(args, ts(*q.After))
	}
	if len(where) == 0 {
		return "1=1", args
	}
	return strings.Join(where, " AND "), args
}

// matchExpr builds a safe FTS5 expression: each alphanumeric token (len>1)
// becomes a quoted OR term, so user text cannot inject FTS operators.
func matchExpr(text string) string {
	var terms []string
	for _, t := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if len(t) > 1 {
			terms = append(terms, `"`+t+`"`)
		}
	}
	return strings.Join(terms, " OR ")
}

// ProjectBoost multiplies the score of a record from the query's own
// project over a global one, so local memory wins ties.
const ProjectBoost = 1.25

// relevance maps a bm25 rank to (0,1] relative to the best match in the same
// result set. SQLite's bm25 has no fixed scale and collapses towards 0 in a
// small store (a term present in half the documents has IDF ≈ 0), so an
// absolute mapping makes every text hit look irrelevant next to lattice
// hits. Relative to the best candidate, the top match is 1.0 and the rest
// keep their proportion.
func relevance(rank, best float64) float64 {
	if best >= 0 || rank >= 0 {
		return 1
	}
	return rank / best
}

// Score is the retrieval score: relevance × importance × activation (×
// project boost). A pinned record has full activation: it never fades.
func (m *Memory) score(r MemoryRecord, rel float64, project string, now time.Time) ScoreDecomposition {
	d := ScoreDecomposition{
		BM25Normalized:   rel,
		ImportanceOffset: r.Importance,
		Activation:       ActivationScore(r.AccessLog, ParamsFor(r.Tier), now),
		ProjectBoost:     1,
	}
	if r.Pinned {
		d.Activation = 1
	}
	if project != "" && r.Project == project {
		d.ProjectBoost = ProjectBoost
	}
	d.Final = d.BM25Normalized * d.ImportanceOffset * d.Activation * d.ProjectBoost
	return d
}

// Search runs a full-text query and ranks candidates by the retrieval score.
// Surfacing a record does not reinforce it; only Touch does.
func (m *Memory) Search(ctx context.Context, q Query) ([]ScoredMemory, error) {
	expr := matchExpr(q.Text)
	if expr == "" {
		return []ScoredMemory{}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	where, args := scopeSQL(q)
	rows, err := m.db.sql.QueryContext(ctx,
		`SELECT r.json, bm25(memory_fts) AS rank FROM memory_fts
		 JOIN memory_records r ON r.rowid = memory_fts.rowid
		 WHERE memory_fts MATCH ? AND `+where+` ORDER BY rank ASC LIMIT 200`,
		append([]any{expr}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := m.db.Now()
	type cand struct {
		r    MemoryRecord
		rank float64
	}
	var cands []cand
	best := 0.0 // most negative bm25 = best match
	for rows.Next() {
		var blob string
		var rank float64
		if err := rows.Scan(&blob, &rank); err != nil {
			return nil, err
		}
		r, err := decode(blob)
		if err != nil {
			return nil, err
		}
		cands = append(cands, cand{r, rank})
		best = math.Min(best, rank)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []ScoredMemory{}
	for _, c := range cands {
		out = append(out, ScoredMemory{Record: c.r, Score: m.score(c.r, relevance(c.rank, best), q.Project, now)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score.Final > out[j].Score.Final })
	if len(out) > limit {
		out = out[:limit]
	}
	for i := range out {
		if out[i].Record, err = m.fillPaths(ctx, out[i].Record); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// List returns records matching q without scoring, newest first.
func (m *Memory) List(ctx context.Context, q Query) ([]MemoryRecord, error) {
	where, args := scopeSQL(q)
	limit := q.Limit
	if limit <= 0 {
		limit = -1
	}
	rows, err := m.db.sql.QueryContext(ctx,
		`SELECT r.json FROM memory_records r WHERE `+where+` ORDER BY r.pinned DESC, r.created_at DESC LIMIT ?`,
		append(args, limit)...)
	if err != nil {
		return nil, err
	}
	var out []MemoryRecord
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			rows.Close()
			return nil, err
		}
		r, err := decode(blob)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i], err = m.fillPaths(ctx, out[i]); err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []MemoryRecord{}
	}
	return out, nil
}

// Recent returns the newest live episodic-or-session records in scope.
func (m *Memory) Recent(ctx context.Context, q Query) ([]MemoryRecord, error) {
	return m.List(ctx, q)
}

// Touch records that the given memories were actually relied on, reinforcing
// their activation and their lattice links. Surfacing alone never calls this.
func (m *Memory) Touch(ctx context.Context, ids []string) error {
	at := m.db.Now()
	for _, id := range ids {
		err := m.db.tx(ctx, func(t *sql.Tx) error {
			r, rowid, err := m.getQ(ctx, t, id)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return nil
				}
				return err
			}
			r.AccessLog = TouchAccessLog(r.AccessLog, at)
			return m.writeBackQ(ctx, t, rowid, r)
		})
		if err != nil {
			return err
		}
		if err := m.lat.Reinforce(ctx, ItemMemory, id, 0.15); err != nil {
			return err
		}
	}
	return nil
}

// Supersede marks old as replaced by byID. The original is kept.
func (m *Memory) Supersede(ctx context.Context, oldID, byID string) error {
	if oldID == byID {
		return errors.New("a memory cannot supersede itself")
	}
	if _, err := m.Get(ctx, byID); err != nil {
		return err
	}
	return m.mutate(ctx, oldID, func(r *MemoryRecord) { r.SupersededBy = byID })
}

// Unsupersede restores a superseded record to live status.
func (m *Memory) Unsupersede(ctx context.Context, id string) error {
	return m.mutate(ctx, id, func(r *MemoryRecord) { r.SupersededBy = "" })
}

// SetPinned pins or unpins a record. Pinned records never fade or compress.
func (m *Memory) SetPinned(ctx context.Context, id string, pinned bool) error {
	return m.mutate(ctx, id, func(r *MemoryRecord) { r.Pinned = pinned })
}

// Archive soft-deletes a record: hidden from recall, still on disk.
func (m *Memory) Archive(ctx context.Context, id string) error {
	at := m.db.Now()
	return m.mutate(ctx, id, func(r *MemoryRecord) { r.ArchivedAt = &at })
}

// Restore brings an archived record back.
func (m *Memory) Restore(ctx context.Context, id string) error {
	return m.mutate(ctx, id, func(r *MemoryRecord) { r.ArchivedAt = nil })
}

// Delete permanently removes a record and its links. This is a human action
// surfaced only in the UI; agents archive instead.
func (m *Memory) Delete(ctx context.Context, id string) error {
	return m.db.tx(ctx, func(t *sql.Tx) error {
		_, rowid, err := m.getQ(ctx, t, id)
		if err != nil {
			return err
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM memory_fts WHERE rowid = ?`, rowid); err != nil {
			return err
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM memory_records WHERE rowid = ?`, rowid); err != nil {
			return err
		}
		if err := m.lat.unlinkAllQ(ctx, t, ItemMemory, id); err != nil {
			return err
		}
		return m.lat.clearUntaggedQ(ctx, t, ItemMemory, id)
	})
}

// mutate applies a bookkeeping change without re-normalising content.
func (m *Memory) mutate(ctx context.Context, id string, fn func(*MemoryRecord)) error {
	return m.db.tx(ctx, func(t *sql.Tx) error {
		r, rowid, err := m.getQ(ctx, t, id)
		if err != nil {
			return err
		}
		fn(&r)
		r.UpdatedAt = m.db.Now()
		if r.SupersededBy != "" || r.ArchivedAt != nil {
			// A retired memory no longer needs classifying.
			if err := m.lat.clearUntaggedQ(ctx, t, ItemMemory, id); err != nil {
				return err
			}
		}
		return m.writeBackQ(ctx, t, rowid, r)
	})
}

// Stats summarises the store for the status bar and overview.
type Stats struct {
	Total      int `json:"total"`
	Episodic   int `json:"episodic"`
	Semantic   int `json:"semantic"`
	Pinned     int `json:"pinned"`
	Superseded int `json:"superseded"`
	Archived   int `json:"archived"`
	Untagged   int `json:"untagged"`
	Nodes      int `json:"nodes"`
	Links      int `json:"links"`
	Captures   int `json:"capturesPending"`
}

// Stats counts records, lattice and pending captures.
func (m *Memory) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	err := m.db.sql.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(tier='episodic' AND superseded_by IS NULL AND archived_at IS NULL),0),
		COALESCE(SUM(tier='semantic' AND superseded_by IS NULL AND archived_at IS NULL),0),
		COALESCE(SUM(pinned),0),
		COALESCE(SUM(superseded_by IS NOT NULL),0),
		COALESCE(SUM(archived_at IS NOT NULL),0)
		FROM memory_records`).Scan(&s.Total, &s.Episodic, &s.Semantic, &s.Pinned, &s.Superseded, &s.Archived)
	if err != nil {
		return s, err
	}
	if s.Untagged, err = m.lat.UntaggedCount(ctx); err != nil {
		return s, err
	}
	if err = m.db.sql.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM category_nodes), (SELECT COUNT(*) FROM category_links)`).Scan(&s.Nodes, &s.Links); err != nil {
		return s, err
	}
	err = m.db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM capture_sessions WHERE closed_at IS NOT NULL AND summarized_at IS NULL`).Scan(&s.Captures)
	return s, err
}

// ActivationOf returns a record's current activation (pinned = 1), for the UI.
func (m *Memory) ActivationOf(r MemoryRecord) float64 {
	if r.Pinned {
		return 1
	}
	return ActivationScore(r.AccessLog, ParamsFor(r.Tier), m.db.Now())
}

// boolInt stores a bool as SQLite 0/1.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullStr stores "" as NULL.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ProjectCount is a project key and how many live memories it holds.
type ProjectCount struct {
	Project string `json:"project"` // "" = global
	Count   int    `json:"count"`
}

// Projects lists every project that has live memories, busiest first.
func (m *Memory) Projects(ctx context.Context) ([]ProjectCount, error) {
	rows, err := m.db.sql.QueryContext(ctx,
		`SELECT project, COUNT(*) FROM memory_records WHERE superseded_by IS NULL AND archived_at IS NULL
		 GROUP BY project ORDER BY COUNT(*) DESC, project ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectCount{}
	for rows.Next() {
		var p ProjectCount
		if err := rows.Scan(&p.Project, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

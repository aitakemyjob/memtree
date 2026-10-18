package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aitakemyjob/memtree/internal/trace"
)

// traceTS is a fixed-width UTC timestamp, so trace times compare and sort
// correctly as text (RFC3339Nano drops trailing zeros and does not).
const traceTS = "2006-01-02T15:04:05.000000000Z"

// Traces is the SQLite store for retrieval traces. It implements
// trace.Recorder, trace.Reader, trace.UseWriter and trace.Pruner and holds
// no policy of its own.
type Traces struct{ db *DB }

// NewTraces returns the trace store.
func NewTraces(db *DB) *Traces { return &Traces{db: db} }

// fmtTraceTS formats a time for the trace tables.
func fmtTraceTS(t time.Time) string { return t.UTC().Format(traceTS) }

// parseTraceTS parses a trace timestamp; a malformed value yields the zero time.
func parseTraceTS(s string) time.Time {
	t, _ := time.Parse(traceTS, s)
	return t
}

// Record stores an event and its hits in one transaction.
func (t *Traces) Record(ctx context.Context, e trace.Event) error {
	matches, err := json.Marshal(nonNilMatches(e.Matches))
	if err != nil {
		return err
	}
	return t.db.tx(ctx, func(q *sql.Tx) error {
		if _, err := q.ExecContext(ctx, `INSERT INTO retrieval_events
			(id, at, origin, tool, harness, session_id, project, query, matches, reinforced)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, fmtTraceTS(e.At), e.Origin, e.Tool, e.Harness, e.SessionID, e.Project, e.Query, string(matches), boolInt(e.Reinforced)); err != nil {
			return err
		}
		for _, h := range e.Hits {
			f := h.Factors
			if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO retrieval_hits
				(event_id, memory_id, rank, route, via, seed, depth, text_score, link_score, importance, activation, project_boost, both_bonus, final)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				e.ID, h.MemoryID, h.Rank, h.Route, h.Via, h.Seed, h.Depth,
				f.Text, f.Link, f.Importance, f.Activation, f.ProjectBoost, f.BothBonus, f.Final); err != nil {
				return err
			}
		}
		return nil
	})
}

// MarkUsed stores use evidence; repeated evidence is ignored.
func (t *Traces) MarkUsed(ctx context.Context, uses []trace.Use) error {
	return t.db.tx(ctx, func(q *sql.Tx) error {
		for _, u := range uses {
			at := u.At
			if at.IsZero() {
				at = t.db.Now()
			}
			if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO retrieval_use (event_id, memory_id, evidence, detail, at)
				VALUES (?, ?, ?, ?, ?)`, u.EventID, u.MemoryID, u.Evidence, u.Detail, fmtTraceTS(at)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Prune deletes events recorded before the cutoff, with their hits and uses.
func (t *Traces) Prune(ctx context.Context, before time.Time) (int, error) {
	var n int64
	err := t.db.tx(ctx, func(q *sql.Tx) error {
		cut := fmtTraceTS(before)
		old := `SELECT id FROM retrieval_events WHERE at < ?`
		if _, err := q.ExecContext(ctx, `DELETE FROM retrieval_hits WHERE event_id IN (`+old+`)`, cut); err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM retrieval_use WHERE event_id IN (`+old+`)`, cut); err != nil {
			return err
		}
		res, err := q.ExecContext(ctx, `DELETE FROM retrieval_events WHERE at < ?`, cut)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return int(n), err
}

// eventWhere builds the WHERE clause for f over retrieval_events aliased e.
func eventWhere(f trace.Filter) (string, []any) {
	var conds []string
	var args []any
	if f.Project != "" {
		conds, args = append(conds, "e.project = ?"), append(args, f.Project)
	}
	if len(f.Origins) > 0 {
		conds = append(conds, "e.origin IN (?"+strings.Repeat(", ?", len(f.Origins)-1)+")")
		for _, o := range f.Origins {
			args = append(args, o)
		}
	}
	if f.SessionID != "" {
		conds, args = append(conds, "e.session_id = ?"), append(args, f.SessionID)
	}
	if !f.Since.IsZero() {
		conds, args = append(conds, "e.at >= ?"), append(args, fmtTraceTS(f.Since))
	}
	if !f.Until.IsZero() {
		conds, args = append(conds, "e.at <= ?"), append(args, fmtTraceTS(f.Until))
	}
	if f.MemoryID != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM retrieval_hits x WHERE x.event_id = e.id AND x.memory_id = ?)")
		args = append(args, f.MemoryID)
	}
	if f.QuietOnly {
		conds = append(conds, "NOT EXISTS (SELECT 1 FROM retrieval_hits x WHERE x.event_id = e.id)")
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// eventCols are the retrieval_events columns scanEvent reads, in order.
const eventCols = `e.id, e.at, e.origin, e.tool, e.harness, e.session_id, e.project, e.query, e.matches, e.reinforced`

// scanEvent reads one event row (without hits or uses).
func scanEvent(sc interface{ Scan(...any) error }) (trace.Event, error) {
	var e trace.Event
	var at, matches string
	var reinforced int
	if err := sc.Scan(&e.ID, &at, &e.Origin, &e.Tool, &e.Harness, &e.SessionID, &e.Project, &e.Query, &matches, &reinforced); err != nil {
		return e, err
	}
	e.At, e.Reinforced = parseTraceTS(at), reinforced != 0
	if err := json.Unmarshal([]byte(matches), &e.Matches); err != nil || e.Matches == nil {
		e.Matches = []trace.Match{}
	}
	return e, nil
}

// List returns matching events, newest first, with their hits and uses.
func (t *Traces) List(ctx context.Context, f trace.Filter) ([]trace.Event, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where, args := eventWhere(f)
	rows, err := t.db.sql.QueryContext(ctx, `SELECT `+eventCols+` FROM retrieval_events e`+where+
		` ORDER BY e.at DESC, e.id DESC LIMIT ? OFFSET ?`, append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, err
	}
	out := []trace.Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := t.fill(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Get returns one event with its hits and uses.
func (t *Traces) Get(ctx context.Context, id string) (trace.Event, error) {
	e, err := scanEvent(t.db.sql.QueryRowContext(ctx, `SELECT `+eventCols+` FROM retrieval_events e WHERE e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, fmt.Errorf("%w: retrieval %s", ErrNotFound, id)
	}
	if err != nil {
		return e, err
	}
	return e, t.fill(ctx, &e)
}

// fill loads an event's hits (by rank) and uses.
func (t *Traces) fill(ctx context.Context, e *trace.Event) error {
	rows, err := t.db.sql.QueryContext(ctx, `SELECT memory_id, rank, route, via, seed, depth,
		text_score, link_score, importance, activation, project_boost, both_bonus, final
		FROM retrieval_hits WHERE event_id = ? ORDER BY rank`, e.ID)
	if err != nil {
		return err
	}
	e.Hits = []trace.Hit{}
	for rows.Next() {
		var h trace.Hit
		f := &h.Factors
		if err := rows.Scan(&h.MemoryID, &h.Rank, &h.Route, &h.Via, &h.Seed, &h.Depth,
			&f.Text, &f.Link, &f.Importance, &f.Activation, &f.ProjectBoost, &f.BothBonus, &f.Final); err != nil {
			rows.Close()
			return err
		}
		e.Hits = append(e.Hits, h)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = t.db.sql.QueryContext(ctx, `SELECT memory_id, evidence, detail, at FROM retrieval_use WHERE event_id = ? ORDER BY at, memory_id`, e.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	e.Uses = []trace.Use{}
	for rows.Next() {
		u := trace.Use{EventID: e.ID}
		var at string
		if err := rows.Scan(&u.MemoryID, &u.Evidence, &u.Detail, &at); err != nil {
			return err
		}
		u.At = parseTraceTS(at)
		e.Uses = append(e.Uses, u)
	}
	return rows.Err()
}

// hitAggregate is the per-hit join Heat and Daily aggregate over: each row
// is one memory returned by one matching event, with whether it was used.
func hitAggregate(f trace.Filter) (string, []any) {
	where, args := eventWhere(trace.Filter{Project: f.Project, Origins: f.Origins, SessionID: f.SessionID, Since: f.Since, Until: f.Until})
	q := `SELECT h.memory_id AS memory_id, e.at AS at,
		EXISTS (SELECT 1 FROM retrieval_use u WHERE u.event_id = h.event_id AND u.memory_id = h.memory_id) AS used
		FROM retrieval_hits h JOIN retrieval_events e ON e.id = h.event_id` + where
	if f.MemoryID != "" {
		if where == "" {
			q += " WHERE"
		} else {
			q += " AND"
		}
		q += " h.memory_id = ?"
		args = append(args, f.MemoryID)
	}
	return q, args
}

// Heat aggregates retrievals per memory, most retrieved first.
func (t *Traces) Heat(ctx context.Context, f trace.Filter) ([]trace.HeatRow, error) {
	inner, args := hitAggregate(f)
	rows, err := t.db.sql.QueryContext(ctx, `SELECT memory_id, COUNT(*), SUM(used), MAX(at) FROM (`+inner+`)
		GROUP BY memory_id ORDER BY COUNT(*) DESC, memory_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []trace.HeatRow{}
	for rows.Next() {
		var r trace.HeatRow
		var last string
		if err := rows.Scan(&r.MemoryID, &r.Retrieved, &r.Used, &last); err != nil {
			return nil, err
		}
		r.LastRetrieved = parseTraceTS(last)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Daily aggregates memory retrievals per UTC day, oldest first.
func (t *Traces) Daily(ctx context.Context, f trace.Filter) ([]trace.DayCount, error) {
	inner, args := hitAggregate(f)
	rows, err := t.db.sql.QueryContext(ctx, `SELECT substr(at, 1, 10) AS day, COUNT(*), SUM(used) FROM (`+inner+`)
		GROUP BY day ORDER BY day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []trace.DayCount{}
	for rows.Next() {
		var d trace.DayCount
		if err := rows.Scan(&d.Day, &d.Retrieved, &d.Used); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// nonNilMatches keeps an empty match list from serialising as null.
func nonNilMatches(m []trace.Match) []trace.Match {
	if m == nil {
		return []trace.Match{}
	}
	return m
}

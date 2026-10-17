package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxTurnChars   = 4000 // stored per turn
	digestTurnLen  = 400  // each turn's share of the raw digest
	digestMaxChars = 2400
)

// CaptureSession is one harness conversation memtree is recording via hooks.
type CaptureSession struct {
	ID           string     `json:"id"`
	Harness      string     `json:"harness"`
	Project      string     `json:"project"`
	StartedAt    time.Time  `json:"startedAt"`
	LastAt       time.Time  `json:"lastAt"`
	ClosedAt     *time.Time `json:"closedAt,omitempty"`
	CloseReason  string     `json:"closeReason"`
	RecordID     string     `json:"recordId"` // the raw capture memory
	SummarizedAt *time.Time `json:"summarizedAt,omitempty"`
	Turns        int        `json:"turns"`
}

// CaptureTurn is one captured message.
type CaptureTurn struct {
	Seq  int       `json:"seq"`
	Role string    `json:"role"` // user | assistant
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Captures records hook traffic. memtree runs no model, so a closed session
// becomes a raw "capture" memory (a trimmed digest) that the agent later
// replaces with a real summary.
type Captures struct {
	db  *DB
	mem *Memory
}

// NewCaptures returns the capture store.
func NewCaptures(db *DB, mem *Memory) *Captures { return &Captures{db: db, mem: mem} }

// AddTurn appends a turn, creating the session on first sight and reopening
// it if it had been closed. Empty text is ignored.
func (c *Captures) AddTurn(ctx context.Context, harness, sessionID, project, role, text string) error {
	text = strings.TrimSpace(text)
	if sessionID == "" || text == "" {
		return nil
	}
	if role != "user" && role != "assistant" {
		return fmt.Errorf("unknown role %q", role)
	}
	at := ts(c.db.Now())
	return c.db.tx(ctx, func(t *sql.Tx) error {
		if _, err := t.ExecContext(ctx,
			`INSERT INTO capture_sessions (id, harness, project, started_at, last_at) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET last_at = excluded.last_at, closed_at = NULL,
			   project = CASE WHEN project = '' THEN excluded.project ELSE project END`,
			sessionID, harness, project, at, at); err != nil {
			return err
		}
		var seq int
		if err := t.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM capture_turns WHERE session_id = ?`, sessionID).Scan(&seq); err != nil {
			return err
		}
		_, err := t.ExecContext(ctx, `INSERT INTO capture_turns (session_id, seq, role, text, at) VALUES (?, ?, ?, ?, ?)`,
			sessionID, seq, role, truncate(text, maxTurnChars), at)
		return err
	})
}

// Close ends a session and writes (or refreshes) its raw capture memory.
// It returns the capture record, or nil if the session had no turns. It is
// idempotent: closing twice refreshes the same record.
func (c *Captures) Close(ctx context.Context, sessionID, reason string) (*MemoryRecord, error) {
	s, err := c.Session(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	turns, err := c.Turns(ctx, sessionID)
	if err != nil || len(turns) == 0 {
		return nil, err
	}
	at := ts(c.db.Now())
	if _, err := c.db.sql.ExecContext(ctx, `UPDATE capture_sessions SET closed_at = ?, close_reason = ? WHERE id = ?`, at, reason, sessionID); err != nil {
		return nil, err
	}
	if s.SummarizedAt != nil {
		return nil, nil // the agent already summarised it; nothing raw to keep
	}
	digest := Digest(turns)
	var rec MemoryRecord
	if s.RecordID != "" {
		rec, err = c.mem.Update(ctx, s.RecordID, func(r *MemoryRecord) { r.Summary = digest }, nil)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err == nil {
			return &rec, nil
		}
	}
	rec, err = c.mem.Save(ctx, MemoryRecord{
		Project: s.Project, Harness: s.Harness, SessionID: sessionID,
		Kind: KindCapture, Summary: digest, Importance: ImportanceRoutine, Provenance: ProvInferred,
	})
	if err != nil {
		return nil, err
	}
	if _, err := c.db.sql.ExecContext(ctx, `UPDATE capture_sessions SET record_id = ? WHERE id = ?`, rec.ID, sessionID); err != nil {
		return nil, err
	}
	return &rec, nil
}

// MarkSummarized records that the agent wrote a real summary for the session
// (rec) and retires the raw capture, which is superseded, not deleted.
func (c *Captures) MarkSummarized(ctx context.Context, sessionID, summaryID string) error {
	s, err := c.Session(ctx, sessionID)
	if err != nil {
		return err
	}
	if _, err := c.db.sql.ExecContext(ctx, `UPDATE capture_sessions SET summarized_at = ? WHERE id = ?`, ts(c.db.Now()), sessionID); err != nil {
		return err
	}
	if s.RecordID != "" && s.RecordID != summaryID {
		if err := c.mem.mutate(ctx, s.RecordID, func(r *MemoryRecord) { r.SupersededBy = summaryID }); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// Session returns one capture session.
func (c *Captures) Session(ctx context.Context, id string) (CaptureSession, error) {
	list, err := c.query(ctx, `WHERE s.id = ?`, id)
	if err != nil {
		return CaptureSession{}, err
	}
	if len(list) == 0 {
		return CaptureSession{}, fmt.Errorf("%w: capture session %s", ErrNotFound, id)
	}
	return list[0], nil
}

// Pending lists closed sessions the agent has not yet summarised, newest
// first. project scopes it ("" with all=false = global only).
func (c *Captures) Pending(ctx context.Context, project string, all bool, limit int) ([]CaptureSession, error) {
	where := `WHERE s.closed_at IS NOT NULL AND s.summarized_at IS NULL`
	var args []any
	if !all {
		where += ` AND (s.project = '' OR s.project = ?)`
		args = append(args, project)
	}
	where += ` AND EXISTS (SELECT 1 FROM capture_turns t WHERE t.session_id = s.id)`
	list, err := c.query(ctx, where, args...)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// Recent lists all capture sessions newest first.
func (c *Captures) Recent(ctx context.Context, limit int) ([]CaptureSession, error) {
	list, err := c.query(ctx, ``)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

// query loads capture sessions matching a WHERE clause, newest first.
func (c *Captures) query(ctx context.Context, where string, args ...any) ([]CaptureSession, error) {
	rows, err := c.db.sql.QueryContext(ctx,
		`SELECT s.id, s.harness, s.project, s.started_at, s.last_at, s.closed_at, s.close_reason, s.record_id, s.summarized_at,
		        (SELECT COUNT(*) FROM capture_turns t WHERE t.session_id = s.id)
		 FROM capture_sessions s `+where+` ORDER BY s.last_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaptureSession{}
	for rows.Next() {
		var s CaptureSession
		var started, last string
		var closed, summarized sql.NullString
		if err := rows.Scan(&s.ID, &s.Harness, &s.Project, &started, &last, &closed, &s.CloseReason, &s.RecordID, &summarized, &s.Turns); err != nil {
			return nil, err
		}
		s.StartedAt, s.LastAt = parseTS(started), parseTS(last)
		if closed.Valid {
			t := parseTS(closed.String)
			s.ClosedAt = &t
		}
		if summarized.Valid {
			t := parseTS(summarized.String)
			s.SummarizedAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Turns returns a session's turns in order.
func (c *Captures) Turns(ctx context.Context, sessionID string) ([]CaptureTurn, error) {
	rows, err := c.db.sql.QueryContext(ctx, `SELECT seq, role, text, at FROM capture_turns WHERE session_id = ? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaptureTurn{}
	for rows.Next() {
		var t CaptureTurn
		var at string
		if err := rows.Scan(&t.Seq, &t.Role, &t.Text, &at); err != nil {
			return nil, err
		}
		t.At = parseTS(at)
		out = append(out, t)
	}
	return out, rows.Err()
}

// Digest builds the raw capture summary from a transcript: the opening user
// prompts and the final assistant message, each trimmed, capped overall.
// It is not a summary, just enough text to find the session by keyword until
// the agent writes a real one.
func Digest(turns []CaptureTurn) string {
	var parts []string
	users := 0
	lastAssistant := -1
	for i, t := range turns {
		if t.Role == "assistant" {
			lastAssistant = i
		}
	}
	for i, t := range turns {
		switch {
		case t.Role == "user" && users < 3:
			users++
			parts = append(parts, "User: "+oneLine(t.Text, digestTurnLen))
		case i == lastAssistant:
			parts = append(parts, "Assistant (last): "+oneLine(t.Text, digestTurnLen))
		}
	}
	return truncate(strings.Join(parts, "\n"), digestMaxChars)
}

// oneLine collapses whitespace and shortens s to n runes.
func oneLine(s string, n int) string {
	return truncate(strings.Join(strings.Fields(s), " "), n)
}

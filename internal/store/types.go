package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Memory tiers.
const (
	TierEpisodic = "episodic" // a specific event or session, full detail
	TierSemantic = "semantic" // distilled gist that supersedes episodic records
)

// Provenance says how trustworthy a memory is.
const (
	ProvHuman    = "HUMAN_CONFIRMED"
	ProvSource   = "SOURCE_GROUNDED"
	ProvInferred = "MODEL_INFERRED"
)

// Memory kinds (free-form beyond these; the UI groups by them).
const (
	KindNote     = "note"
	KindDecision = "decision"
	KindLesson   = "lesson"
	KindSession  = "session" // a summarised conversation
	KindCapture  = "capture" // raw hook capture awaiting the agent's summary
)

// Importance values. The value is also the score multiplier.
const (
	ImportanceRoutine  = 1.0
	ImportanceDecision = 1.5
	ImportanceLesson   = 2.5
)

// Lattice item types memtree links. Only "memory" is produced today; the type
// column stays open so other artifact kinds can join the same lattice.
const ItemMemory = "memory"

// Link sources record who placed a lattice link.
const (
	SourceAgent = "agent"
	SourceHuman = "human"
	SourceAuto  = "auto"
)

// MemoryRecord is one stored memory. Agents supply everything but the
// bookkeeping fields (ID, timestamps, AccessLog, SupersededBy, ArchivedAt).
type MemoryRecord struct {
	ID           string      `json:"id"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
	Project      string      `json:"project"` // repo path; "" = global
	Harness      string      `json:"harness"` // claude-code, codex, cursor, vscode, mcp, ui…
	SessionID    string      `json:"sessionId,omitempty"`
	Tier         string      `json:"tier"`
	Kind         string      `json:"kind"`
	Summary      string      `json:"summary"`
	Keywords     []string    `json:"keywords"`
	Decisions    []string    `json:"decisions"`
	OpenLoops    []string    `json:"openLoops"`
	Importance   float64     `json:"importance"`
	Provenance   string      `json:"provenance"`
	Pinned       bool        `json:"pinned"`
	AccessLog    []time.Time `json:"accessLog"`
	SupersededBy string      `json:"supersededBy,omitempty"`
	ArchivedAt   *time.Time  `json:"archivedAt,omitempty"`
	// LatticePaths are the category paths the record is linked under. On
	// input they are linked at save; on output they are filled from the lattice.
	LatticePaths []string `json:"latticePaths"`
}

// ScoreDecomposition explains a retrieval score:
// final = max(bm25, link) × importance × activation × project boost × both bonus.
// BM25Normalized is the text route's relevance and LinkScore the lattice
// route's (link weight × node activation); a route that did not find the
// memory leaves its term zero. BothBonus is zero unless both routes agreed.
type ScoreDecomposition struct {
	BM25Normalized   float64 `json:"bm25Normalized"`
	LinkScore        float64 `json:"linkScore,omitempty"`
	ImportanceOffset float64 `json:"importanceOffset"`
	Activation       float64 `json:"activation"`
	ProjectBoost     float64 `json:"projectBoost"`
	BothBonus        float64 `json:"bothBonus,omitempty"`
	Final            float64 `json:"final"`
}

// ScoredMemory is a search result.
type ScoredMemory struct {
	Record MemoryRecord       `json:"record"`
	Score  ScoreDecomposition `json:"score"`
}

// Query scopes a search or listing.
type Query struct {
	Text              string
	Limit             int
	Before            *time.Time
	After             *time.Time
	Tier              string
	Kind              string
	ExcludeKind       string // hide one kind, e.g. raw captures
	Project           string // "" with AllProjects=false means global-only
	AllProjects       bool   // ignore project scoping entirely
	IncludeSuperseded bool
	IncludeArchived   bool
	PinnedOnly        bool
}

// InScope reports whether a record belongs in a query scoped to project:
// global records (project "") are visible everywhere, project records only
// in their own project. allProjects disables the filter.
func InScope(recordProject, project string, allProjects bool) bool {
	if allProjects || recordProject == "" {
		return true
	}
	return recordProject == project
}

// querier is satisfied by both *sql.DB and *sql.Tx so helpers can run inside
// or outside a transaction. With a single pooled connection, code inside a
// transaction must never touch the *sql.DB directly.
type querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// tx runs fn in a transaction, committing on nil and rolling back otherwise.
func (d *DB) tx(ctx context.Context, fn func(q *sql.Tx) error) error {
	t, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

// nonNil keeps slices from serialising as JSON null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// cleanStrings trims, drops empties and de-duplicates (order preserved).
func cleanStrings(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

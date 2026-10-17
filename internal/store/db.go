// Package store is memtree's persistence layer: one SQLite database holding
// memory records (with an FTS5 index), the category lattice, and hook
// captures. It is a port of cortex's memory and lattice stores; every type
// takes an injected clock so time-based behaviour is testable.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite with FTS5
)

// DB is the shared database handle every store in this package uses.
type DB struct {
	sql *sql.DB
	now func() time.Time
}

// Open opens (creating if needed) the database at path. Use ":memory:" for
// tests. now is the clock; nil means time.Now.
func Open(path string, now func() time.Time) (*DB, error) {
	if now == nil {
		now = time.Now
	}
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite serialises writers anyway, and an in-memory
	// database exists per connection.
	db.SetMaxOpenConns(1)
	d := &DB{sql: db, now: now}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// Now returns the store clock's current time in UTC.
func (d *DB) Now() time.Time { return d.now().UTC() }

// migrate creates every table idempotently.
func (d *DB) migrate() error {
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS memory_records (
	rowid INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT UNIQUE NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	project TEXT NOT NULL DEFAULT '',
	harness TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	tier TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'note',
	importance REAL NOT NULL,
	pinned INTEGER NOT NULL DEFAULT 0,
	superseded_by TEXT,
	archived_at TEXT,
	json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS memory_live ON memory_records(superseded_by, archived_at, created_at);
CREATE INDEX IF NOT EXISTS memory_project ON memory_records(project);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
	summary, keywords, decisions, tokenize='porter unicode61'
);
CREATE TABLE IF NOT EXISTS category_nodes (
	id TEXT PRIMARY KEY,
	parent_id TEXT,
	label TEXT NOT NULL,
	created_at TEXT NOT NULL,
	access_count INTEGER NOT NULL DEFAULT 0,
	last_accessed TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_parent ON category_nodes(parent_id);
CREATE TABLE IF NOT EXISTS category_links (
	item_type TEXT NOT NULL,
	item_id TEXT NOT NULL,
	category_id TEXT NOT NULL,
	weight REAL NOT NULL,
	summary TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'agent',
	created_at TEXT NOT NULL,
	last_accessed TEXT NOT NULL,
	PRIMARY KEY (item_type, item_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_links_category ON category_links(category_id);
CREATE TABLE IF NOT EXISTS lattice_untagged (
	item_type TEXT NOT NULL,
	item_id TEXT NOT NULL,
	text TEXT NOT NULL,
	at TEXT NOT NULL,
	PRIMARY KEY (item_type, item_id)
);
CREATE TABLE IF NOT EXISTS capture_sessions (
	id TEXT PRIMARY KEY,
	harness TEXT NOT NULL,
	project TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	last_at TEXT NOT NULL,
	closed_at TEXT,
	close_reason TEXT NOT NULL DEFAULT '',
	record_id TEXT NOT NULL DEFAULT '',
	summarized_at TEXT
);
CREATE TABLE IF NOT EXISTS capture_turns (
	session_id TEXT NOT NULL,
	seq INTEGER NOT NULL,
	role TEXT NOT NULL,
	text TEXT NOT NULL,
	at TEXT NOT NULL,
	PRIMARY KEY (session_id, seq)
);
`)
	return err
}

// ts formats a time for storage.
func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// parseTS parses a stored time; a malformed value yields the zero time.
func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// nullTS formats an optional time as a nullable column value.
func nullTS(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

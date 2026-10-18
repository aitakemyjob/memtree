// Package core is memtree's application service. The desktop shell, the
// browser API, the MCP server and the harness hooks are thin adapters over
// it, so they all see the same memory through the same rules.
package core

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/events"
	"github.com/aitakemyjob/memtree/internal/store"
	"github.com/aitakemyjob/memtree/internal/trace"
)

// Event types published on the bus.
const (
	EvConfig   = "config"
	EvActivity = "activity"
	EvMemory   = "memory"  // a memory was saved/changed; Data is {id, op}
	EvLattice  = "lattice" // nodes or links changed
	EvCapture  = "capture" // hook traffic changed a capture session
	EvMCP      = "mcp"
	EvFocus    = "focus" // an agent asked the UI to show something
	EvTrace    = "trace" // a retrieval was recorded or marked used; Data is {op, id}
)

// ErrAgentWritesDisabled is returned to agents when writes are switched off.
var ErrAgentWritesDisabled = errors.New("agent writes are disabled in memtree Settings")

// Activity is one entry in the live feed of agent and hook calls.
type Activity struct {
	ID         int       `json:"id"`
	Time       time.Time `json:"time"`
	Source     string    `json:"source"` // mcp | hook | ui
	Harness    string    `json:"harness"`
	Tool       string    `json:"tool"`
	Summary    string    `json:"summary"`
	Project    string    `json:"project"`
	Error      string    `json:"error,omitempty"`
	DurationMS int64     `json:"durationMs"`
}

// MCPStatus is whether the loopback server is up.
type MCPStatus struct {
	Running bool   `json:"running"`
	URL     string `json:"url"`
	Error   string `json:"error,omitempty"`
}

const activityCap = 300

// Service owns the database and the rules around it.
type Service struct {
	Config  *config.Store
	Bus     *events.Bus
	DB      *store.DB
	Lattice *store.Lattice
	Memory  *store.Memory
	Capture *store.Captures
	Version string

	// Trace records retrievals; Traces reads them back; Uses turns later
	// agent activity into use evidence; Retention prunes old traces.
	Trace     trace.Recorder
	Traces    trace.Reader
	Uses      *trace.UseTracker
	Retention trace.Retention

	mu       sync.Mutex
	activity []Activity
	nextID   int
	mcp      MCPStatus
}

// New opens the database named by the settings and seeds the lattice axes.
func New(cfg *config.Store, version string) (*Service, error) {
	path := cfg.Get().DBPath
	if path == "" {
		p, err := config.DefaultDBPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return Open(cfg, path, version, nil)
}

// Open is New with an explicit database path and clock (":memory:" in tests).
func Open(cfg *config.Store, dbPath, version string, now func() time.Time) (*Service, error) {
	db, err := store.Open(dbPath, now)
	if err != nil {
		return nil, err
	}
	lat := store.NewLattice(db)
	if err := lat.SeedAxes(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	mem := store.NewMemory(db, lat)
	s := &Service{
		Config: cfg, Bus: events.NewBus(), DB: db, Lattice: lat, Memory: mem,
		Capture: store.NewCaptures(db, mem), Version: version,
	}
	s.wireTraces(store.NewTraces(db))
	return s, nil
}

// Close closes the database.
func (s *Service) Close() error { return s.DB.Close() }

// emit publishes an event to the UI bus.
func (s *Service) emit(typ string, data any) { s.Bus.Publish(typ, data) }

// RecordActivity appends to the live feed and publishes it.
func (s *Service) RecordActivity(a Activity) {
	s.mu.Lock()
	s.nextID++
	a.ID = s.nextID
	if a.Time.IsZero() {
		a.Time = time.Now()
	}
	s.activity = append(s.activity, a)
	if len(s.activity) > activityCap {
		s.activity = s.activity[len(s.activity)-activityCap:]
	}
	s.mu.Unlock()
	s.emit(EvActivity, a)
}

// ActivityLog returns the feed, oldest first.
func (s *Service) ActivityLog() []Activity {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Activity{}, s.activity...)
}

// SetMCPStatus records and publishes the server state.
func (s *Service) SetMCPStatus(st MCPStatus) {
	s.mu.Lock()
	s.mcp = st
	s.mu.Unlock()
	s.emit(EvMCP, st)
}

// MCPStatus returns the last published server state.
func (s *Service) MCPStatus() MCPStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mcp
}

// UpdateSettings changes and persists settings and tells the UI.
func (s *Service) UpdateSettings(fn func(*config.Settings)) (config.Settings, error) {
	st, err := s.Config.Update(fn)
	if err == nil {
		s.emit(EvConfig, st)
	}
	return st, err
}

// Maintain runs periodic housekeeping: it ages lattice links, dropping faded
// ones. Nodes and memories are never deleted here. Returns links dropped.
func (s *Service) Maintain(ctx context.Context) (int, error) {
	n, err := s.Lattice.DecayLinks(ctx, s.Config.Get().LinkHalfLifeDays, 0.08)
	if err == nil && n > 0 {
		s.emit(EvLattice, map[string]any{"op": "decay", "dropped": n})
	}
	if err == nil {
		err = s.PruneTraces(ctx)
	}
	return n, err
}

// RunMaintenance calls Maintain on the configured interval until ctx ends.
func (s *Service) RunMaintenance(ctx context.Context) {
	_ = s.PruneTraces(ctx)
	for {
		h := s.Config.Get().MaintainHours
		if h <= 0 {
			h = 12
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(h * float64(time.Hour))):
			_, _ = s.Maintain(ctx)
		}
	}
}

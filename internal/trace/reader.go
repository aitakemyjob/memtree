package trace

import (
	"context"
	"time"
)

// Filter scopes a trace query. Zero fields do not filter.
type Filter struct {
	Project   string    `json:"project"`
	Origins   []string  `json:"origins"`
	SessionID string    `json:"sessionId"`
	MemoryID  string    `json:"memoryId"` // events that returned this memory
	Since     time.Time `json:"since"`
	Until     time.Time `json:"until"`
	QuietOnly bool      `json:"quietOnly"` // events that returned nothing
	Limit     int       `json:"limit"`
	Offset    int       `json:"offset"`
}

// HeatRow is how often one memory was retrieved and used.
type HeatRow struct {
	MemoryID      string    `json:"memoryId"`
	Retrieved     int       `json:"retrieved"`
	Used          int       `json:"used"` // retrievals with any use evidence
	LastRetrieved time.Time `json:"lastRetrieved"`
}

// DayCount is the number of memory retrievals on one UTC day.
type DayCount struct {
	Day       string `json:"day"` // YYYY-MM-DD
	Retrieved int    `json:"retrieved"`
	Used      int    `json:"used"`
}

// Reader queries recorded traces.
type Reader interface {
	// List returns matching events, newest first, with hits and uses.
	List(ctx context.Context, f Filter) ([]Event, error)
	// Get returns one event with its hits and uses.
	Get(ctx context.Context, id string) (Event, error)
	// Heat aggregates retrievals per memory.
	Heat(ctx context.Context, f Filter) ([]HeatRow, error)
	// Daily aggregates memory retrievals per day, oldest first.
	Daily(ctx context.Context, f Filter) ([]DayCount, error)
}

// UseWriter persists use evidence. Recording the same evidence twice is a no-op.
type UseWriter interface {
	MarkUsed(ctx context.Context, uses []Use) error
}

// Pruner deletes traces recorded before a cutoff and reports how many events went.
type Pruner interface {
	Prune(ctx context.Context, before time.Time) (int, error)
}

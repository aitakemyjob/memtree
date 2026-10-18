// Package trace records why memories were retrieved and whether they were
// then used. It holds the domain model and the narrow interfaces the rest
// of memtree depends on; storage, wiring and presentation live elsewhere,
// so a new retrieval source, evidence rule or view plugs in without
// changing this package's callers.
package trace

import "time"

// Origins: who asked for a retrieval.
const (
	OriginMCP      = "mcp"      // an agent tool call
	OriginHook     = "hook"     // a harness hook injecting context
	OriginUI       = "ui"       // the desktop or browser UI
	OriginCLI      = "cli"      // the memtree recall command
	OriginInternal = "internal" // no adapter attributed it
)

// Routes: how a hit was found.
const (
	RouteText    = "text"    // full-text match on the query wording
	RouteLattice = "lattice" // spreading activation from fired categories
	RouteBoth    = "both"    // both routes agreed
	RouteAnchor  = "anchor"  // injected at session start (Via says why: latest, landmark, pinned)
)

// Evidence kinds: why a retrieved memory is believed to have been used.
const (
	EvidenceFetched = "fetched" // the agent read it in full after retrieval
	EvidenceActed   = "acted"   // the agent changed it (supersede, update, pin, compress)
	EvidenceCited   = "cited"   // the agent's reply mentioned its id
)

// Match is one query token that fired a lattice category.
type Match struct {
	Token    string `json:"token"`
	Category string `json:"category"`
}

// Factors are the terms of a hit's score. Final is
// max(Text, Link) × Importance × Activation × ProjectBoost × BothBonus;
// a route that did not contribute has a zero relevance term.
type Factors struct {
	Text         float64 `json:"text"`         // relative BM25 relevance
	Link         float64 `json:"link"`         // link weight × node activation
	Importance   float64 `json:"importance"`   // record importance
	Activation   float64 `json:"activation"`   // record recency/frequency decay
	ProjectBoost float64 `json:"projectBoost"` // same-project multiplier
	BothBonus    float64 `json:"bothBonus"`    // agreement multiplier (1 when one route)
	Final        float64 `json:"final"`
}

// Hit is one memory a retrieval returned, with how it was reached.
type Hit struct {
	MemoryID string  `json:"memoryId"`
	Rank     int     `json:"rank"` // 1-based position in the result
	Route    string  `json:"route"`
	Via      string  `json:"via,omitempty"`  // category the lattice reached it through
	Seed     string  `json:"seed,omitempty"` // fired category the path started at
	Depth    int     `json:"depth"`          // hops from Seed to Via
	Factors  Factors `json:"factors"`
}

// Use is evidence that a retrieved memory was relied on.
type Use struct {
	EventID  string    `json:"eventId"`
	MemoryID string    `json:"memoryId"`
	Evidence string    `json:"evidence"`
	Detail   string    `json:"detail,omitempty"` // e.g. the tool name
	At       time.Time `json:"at"`
}

// Event is one retrieval: who asked, for what, and what came back.
type Event struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	Origin     string    `json:"origin"`
	Tool       string    `json:"tool"` // tool, hook event or API method name
	Harness    string    `json:"harness"`
	SessionID  string    `json:"sessionId"`
	Project    string    `json:"project"`
	Query      string    `json:"query"`
	Matches    []Match   `json:"matches"`
	Reinforced bool      `json:"reinforced"` // the hits were touched as relied on
	Hits       []Hit     `json:"hits"`
	Uses       []Use     `json:"uses"`
}

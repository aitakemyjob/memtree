// Package api is the UI's JSON method table: one entry point, Invoke, used
// by the Wails desktop bridge and the browser's /api/invoke alike. It only
// translates; behaviour lives in core.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"time"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/core"
	"github.com/aitakemyjob/memtree/internal/store"
)

type handler func(ctx context.Context, raw json.RawMessage) (any, error)

// API dispatches named methods to the core service.
type API struct {
	core    *core.Service
	methods map[string]handler
	// PickDirectory opens a native folder chooser; nil in browser mode.
	PickDirectory func() (string, error)
	// Hooks is set by the hooks package so the UI can show and install
	// harness integrations. Nil means the methods report "unavailable".
	Hooks HookManager
}

// HookManager is what the UI needs from the hooks package.
type HookManager interface {
	Status(ctx context.Context) (any, error)
	Install(ctx context.Context, harness string, project string, dryRun bool) (any, error)
	Uninstall(ctx context.Context, harness string, project string) (any, error)
	// Handle runs one harness hook event and returns the text to print.
	Handle(ctx context.Context, harness, event string, payload json.RawMessage) (string, error)
}

// method adapts a typed function into a handler, decoding args unless empty.
func method[T any](fn func(context.Context, T) (any, error)) handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in T
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("bad arguments: %w", err)
			}
		}
		return fn(ctx, in)
	}
}

type none struct{}

// AppState is everything the UI needs at startup.
type AppState struct {
	Version    string               `json:"version"`
	Platform   string               `json:"platform"`
	Settings   config.Settings      `json:"settings"`
	MCP        core.MCPStatus       `json:"mcp"`
	Stats      store.Stats          `json:"stats"`
	Projects   []store.ProjectCount `json:"projects"`
	AxisRoots  []string             `json:"axisRoots"`
	ConfigPath string               `json:"configPath"`
	CanPickDir bool                 `json:"canPickDir"`
}

// New builds the method table over c.
func New(c *core.Service) *API {
	a := &API{core: c, methods: map[string]handler{}}
	a.registerApp()
	a.registerMemory()
	a.registerLattice()
	a.registerOps()
	a.registerHooks()
	return a
}

// Invoke calls a method. args is a JSON object (or empty).
func (a *API) Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	h, ok := a.methods[name]
	if !ok {
		return nil, fmt.Errorf("unknown method %q", name)
	}
	out, err := h(ctx, args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// Methods lists the registered method names, sorted.
func (a *API) Methods() []string {
	out := make([]string, 0, len(a.methods))
	for k := range a.methods {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// registerApp adds app-level methods: state, settings, activity, projects, folder picker.
func (a *API) registerApp() {
	c := a.core
	a.methods["app.state"] = method(func(ctx context.Context, _ none) (any, error) {
		st, err := c.Memory.Stats(ctx)
		if err != nil {
			return nil, err
		}
		pr, err := c.Memory.Projects(ctx)
		if err != nil {
			return nil, err
		}
		return AppState{
			Version: c.Version, Platform: runtime.GOOS, Settings: c.Config.Get(), MCP: c.MCPStatus(),
			Stats: st, Projects: pr, AxisRoots: store.AxisRoots(), ConfigPath: c.Config.Path(), CanPickDir: a.PickDirectory != nil,
		}, nil
	})
	a.methods["settings.update"] = method(func(ctx context.Context, in config.Settings) (any, error) {
		return c.UpdateSettings(func(s *config.Settings) {
			// The UI sends the whole settings object; keep fields it cannot edit.
			in.DBPath = s.DBPath
			*s = in
		})
	})
	a.methods["app.pickDirectory"] = method(func(ctx context.Context, _ none) (any, error) {
		if a.PickDirectory == nil {
			return nil, fmt.Errorf("folder picker unavailable in browser mode; type the path")
		}
		dir, err := a.PickDirectory()
		return map[string]string{"path": dir}, err
	})
	a.methods["activity.list"] = method(func(ctx context.Context, _ none) (any, error) { return c.ActivityLog(), nil })
	a.methods["projects.list"] = method(func(ctx context.Context, _ none) (any, error) { return c.Memory.Projects(ctx) })
}

type listIn struct {
	Project           string `json:"project"`
	AllProjects       bool   `json:"allProjects"`
	Tier              string `json:"tier"`
	Kind              string `json:"kind"`
	ExcludeKind       string `json:"excludeKind"`
	Limit             int    `json:"limit"`
	IncludeSuperseded bool   `json:"includeSuperseded"`
	IncludeArchived   bool   `json:"includeArchived"`
	PinnedOnly        bool   `json:"pinnedOnly"`
	Before            string `json:"before"`
	After             string `json:"after"`
}

// query converts list arguments into a store query, parsing the time bounds.
func (l listIn) query() (store.Query, error) {
	q := store.Query{
		Project: l.Project, AllProjects: l.AllProjects, Tier: l.Tier, Kind: l.Kind, ExcludeKind: l.ExcludeKind, Limit: l.Limit,
		IncludeSuperseded: l.IncludeSuperseded, IncludeArchived: l.IncludeArchived, PinnedOnly: l.PinnedOnly,
	}
	var err error
	if q.Before, err = parseTime(l.Before); err != nil {
		return q, err
	}
	q.After, err = parseTime(l.After)
	return q, err
}

// parseTime parses an optional RFC 3339 time; "" means no bound.
func parseTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, fmt.Errorf("bad time %q (want RFC 3339)", s)
	}
	return &t, nil
}

type idIn struct {
	ID string `json:"id"`
}

type recallIn struct {
	listIn
	Query     string `json:"query"`
	Reinforce bool   `json:"reinforce"`
}

type saveIn struct {
	store.MemoryRecord
}

type updateIn struct {
	ID           string   `json:"id"`
	Summary      *string  `json:"summary"`
	Keywords     []string `json:"keywords"`
	Decisions    []string `json:"decisions"`
	OpenLoops    []string `json:"openLoops"`
	Importance   *float64 `json:"importance"`
	Kind         *string  `json:"kind"`
	Project      *string  `json:"project"`
	Provenance   *string  `json:"provenance"`
	LatticePaths []string `json:"latticePaths"` // nil = leave links alone
	SetPaths     bool     `json:"setPaths"`     // true = replace links with LatticePaths (even empty)
}

// registerMemory adds the memory methods: list, recall, save, edit, lifecycle and explain.
func (a *API) registerMemory() {
	c := a.core
	a.methods["memory.list"] = method(func(ctx context.Context, in listIn) (any, error) {
		q, err := in.query()
		if err != nil {
			return nil, err
		}
		return c.Memory.List(ctx, q)
	})
	a.methods["memory.get"] = method(func(ctx context.Context, in idIn) (any, error) { return c.Memory.Get(ctx, in.ID) })
	a.methods["memory.recall"] = method(func(ctx context.Context, in recallIn) (any, error) {
		q, err := in.query()
		if err != nil {
			return nil, err
		}
		res, err := c.Recall(ctx, core.RecallInput{
			Query: in.Query, Project: in.Project, AllProjects: in.AllProjects, Limit: in.Limit,
			Before: q.Before, After: q.After, Tier: in.Tier, Kind: in.Kind,
			IncludeSuperseded: in.IncludeSuperseded, Reinforce: in.Reinforce,
		})
		if res.Memories == nil {
			res.Memories = []core.Recalled{}
		}
		return res, err
	})
	a.methods["memory.save"] = method(func(ctx context.Context, in saveIn) (any, error) {
		if in.Harness == "" {
			in.Harness = "ui"
		}
		if in.Provenance == "" {
			in.Provenance = store.ProvHuman // typed in by a person
		}
		return c.Save(ctx, in.MemoryRecord)
	})
	a.methods["memory.update"] = method(func(ctx context.Context, in updateIn) (any, error) {
		var paths []string
		if in.SetPaths {
			paths = in.LatticePaths
			if paths == nil {
				paths = []string{}
			}
		}
		return c.Update(ctx, in.ID, func(r *store.MemoryRecord) {
			if in.Summary != nil {
				r.Summary = *in.Summary
			}
			if in.Keywords != nil {
				r.Keywords = in.Keywords
			}
			if in.Decisions != nil {
				r.Decisions = in.Decisions
			}
			if in.OpenLoops != nil {
				r.OpenLoops = in.OpenLoops
			}
			if in.Importance != nil {
				r.Importance = *in.Importance
			}
			if in.Kind != nil {
				r.Kind = *in.Kind
			}
			if in.Project != nil {
				r.Project = *in.Project
			}
			if in.Provenance != nil {
				r.Provenance = *in.Provenance
			}
		}, paths)
	})
	a.methods["memory.pin"] = method(func(ctx context.Context, in struct {
		ID     string `json:"id"`
		Pinned bool   `json:"pinned"`
	}) (any, error) {
		return ok(c.SetPinned(ctx, in.ID, in.Pinned))
	})
	a.methods["memory.archive"] = method(func(ctx context.Context, in idIn) (any, error) { return ok(c.Archive(ctx, in.ID)) })
	a.methods["memory.restore"] = method(func(ctx context.Context, in idIn) (any, error) { return ok(c.Restore(ctx, in.ID)) })
	a.methods["memory.delete"] = method(func(ctx context.Context, in idIn) (any, error) { return ok(c.Delete(ctx, in.ID)) })
	a.methods["memory.supersede"] = method(func(ctx context.Context, in struct {
		ID, By string
	}) (any, error) {
		return ok(c.Supersede(ctx, in.ID, in.By))
	})
	a.methods["memory.unsupersede"] = method(func(ctx context.Context, in idIn) (any, error) { return ok(c.Unsupersede(ctx, in.ID)) })
	a.methods["memory.lineage"] = method(func(ctx context.Context, in idIn) (any, error) { return c.Memory.Lineage(ctx, in.ID) })
	a.methods["memory.touch"] = method(func(ctx context.Context, in struct{ IDs []string }) (any, error) { return ok(c.Touch(ctx, in.IDs)) })
	a.methods["memory.explain"] = method(func(ctx context.Context, in struct {
		ID      string `json:"id"`
		Project string `json:"project"`
	}) (any, error) {
		r, err := c.Memory.Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"record": r, "activation": c.Memory.ActivationOf(r),
			"ageHours":     c.DB.Now().Sub(r.CreatedAt).Hours(),
			"compressible": store.ShouldCompress(r, c.Memory.ActivationOf(r), c.DB.Now(), c.Config.Get().Tiers),
			"links":        mustLinks(c, ctx, in.ID),
		}, nil
	})
}

// mustLinks returns a memory's lattice links, or an empty list on error.
func mustLinks(c *core.Service, ctx context.Context, id string) []store.CategoryLink {
	l, err := c.Lattice.LinksForItem(ctx, store.ItemMemory, id)
	if err != nil || l == nil {
		return []store.CategoryLink{}
	}
	return l
}

// ok turns an error into the {"ok": true} reply or the error.
func ok(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

// registerLattice adds the lattice methods: nodes, links, tagging, merge, prune and probe.
func (a *API) registerLattice() {
	c := a.core
	a.methods["lattice.nodes"] = method(func(ctx context.Context, _ none) (any, error) { return c.Lattice.Nodes(ctx) })
	a.methods["lattice.links"] = method(func(ctx context.Context, in struct{ Category string }) (any, error) {
		return c.Lattice.LinksFor(ctx, in.Category)
	})
	a.methods["lattice.untagged"] = method(func(ctx context.Context, in struct{ Limit int }) (any, error) {
		return c.Lattice.Untagged(ctx, in.Limit)
	})
	a.methods["lattice.tag"] = method(func(ctx context.Context, in struct {
		ID    string   `json:"id"`
		Paths []string `json:"paths"`
	}) (any, error) {
		return ok(c.TagMemory(ctx, in.ID, in.Paths, store.SourceHuman))
	})
	a.methods["lattice.merge"] = method(func(ctx context.Context, in struct{ From, To string }) (any, error) {
		return ok(c.MergeCategory(ctx, in.From, in.To))
	})
	a.methods["lattice.prune"] = method(func(ctx context.Context, _ none) (any, error) {
		n, err := c.PruneCategories(ctx)
		return map[string]int{"removed": n}, err
	})
	a.methods["lattice.lint"] = method(func(ctx context.Context, _ none) (any, error) { return c.Memory.Lint(ctx) })
	a.methods["lattice.checkPaths"] = method(func(ctx context.Context, in struct {
		Paths []string `json:"paths"`
	}) (any, error) {
		return c.Lattice.CheckPaths(ctx, in.Paths)
	})
	a.methods["lattice.ensure"] = method(func(ctx context.Context, in struct{ Path string }) (any, error) {
		id, err := c.Lattice.EnsureNode(ctx, in.Path)
		return map[string]string{"id": id}, err
	})
	// lattice.fire shows what a piece of text lights up, for the UI's "try a prompt" probe.
	a.methods["lattice.fire"] = method(func(ctx context.Context, in struct {
		Text string `json:"text"`
		Max  int    `json:"max"`
	}) (any, error) {
		seeds, hits, err := c.Probe(ctx, in.Text, max(in.Max, 20))
		return map[string]any{"seeds": seeds, "hits": nilHits(hits)}, err
	})
}

// nilHits keeps an activation result from serialising as null.
func nilHits(h []store.ActivationHit) []store.ActivationHit {
	if h == nil {
		return []store.ActivationHit{}
	}
	return h
}

// registerOps adds consolidation and capture methods.
func (a *API) registerOps() {
	c := a.core
	a.methods["consolidation.candidates"] = method(func(ctx context.Context, in struct {
		Project     string `json:"project"`
		AllProjects bool   `json:"allProjects"`
	}) (any, error) {
		return c.Memory.Candidates(ctx, c.Config.Get().Tiers, in.Project, in.AllProjects)
	})
	a.methods["consolidation.compress"] = method(func(ctx context.Context, in struct {
		IDs  []string           `json:"ids"`
		Gist store.MemoryRecord `json:"gist"`
	}) (any, error) {
		if in.Gist.Harness == "" {
			in.Gist.Harness = "ui"
		}
		return c.Compress(ctx, in.IDs, in.Gist)
	})
	a.methods["captures.list"] = method(func(ctx context.Context, in struct{ Limit int }) (any, error) {
		return c.Capture.Recent(ctx, in.Limit)
	})
	a.methods["captures.pending"] = method(func(ctx context.Context, in struct {
		Project     string `json:"project"`
		AllProjects bool   `json:"allProjects"`
	}) (any, error) {
		return c.Capture.Pending(ctx, in.Project, in.AllProjects, 0)
	})
	a.methods["captures.turns"] = method(func(ctx context.Context, in struct {
		Session string `json:"session"`
	}) (any, error) {
		return c.Capture.Turns(ctx, in.Session)
	})
	a.methods["captures.close"] = method(func(ctx context.Context, in struct {
		Session string `json:"session"`
	}) (any, error) {
		return ok(c.CloseSession(ctx, in.Session, "ui"))
	})
}

// registerHooks adds harness integration methods, backed by the hooks manager when present.
func (a *API) registerHooks() {
	unavailable := func() (any, error) { return nil, fmt.Errorf("hook management is unavailable in this build") }
	a.methods["hooks.status"] = method(func(ctx context.Context, _ none) (any, error) {
		if a.Hooks == nil {
			return unavailable()
		}
		return a.Hooks.Status(ctx)
	})
	a.methods["hooks.install"] = method(func(ctx context.Context, in struct {
		Harness string `json:"harness"`
		Project string `json:"project"`
		DryRun  bool   `json:"dryRun"`
	}) (any, error) {
		if a.Hooks == nil {
			return unavailable()
		}
		return a.Hooks.Install(ctx, in.Harness, in.Project, in.DryRun)
	})
	a.methods["hooks.uninstall"] = method(func(ctx context.Context, in struct {
		Harness string `json:"harness"`
		Project string `json:"project"`
	}) (any, error) {
		if a.Hooks == nil {
			return unavailable()
		}
		return a.Hooks.Uninstall(ctx, in.Harness, in.Project)
	})
	// hooks.handle is what `memtree hook` calls on a running service so the
	// UI sees hook traffic live. It returns the text the hook should print.
	a.methods["hooks.handle"] = method(func(ctx context.Context, in struct {
		Harness string          `json:"harness"`
		Event   string          `json:"event"`
		Payload json.RawMessage `json:"payload"`
	}) (any, error) {
		if a.Hooks == nil {
			return unavailable()
		}
		text, err := a.Hooks.Handle(ctx, in.Harness, in.Event, in.Payload)
		return map[string]string{"output": text}, err
	})
}

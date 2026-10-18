package api

import (
	"context"

	"github.com/aitakemyjob/memtree/internal/trace"
)

// registerTrace adds the read-only retrieval trace methods. They depend
// only on trace.Reader, so the UI sees whatever the recorder stored.
func (a *API) registerTrace() {
	r := func() trace.Reader { return a.core.Traces }
	a.methods["trace.list"] = method(func(ctx context.Context, f trace.Filter) (any, error) { return r().List(ctx, f) })
	a.methods["trace.get"] = method(func(ctx context.Context, in idIn) (any, error) { return r().Get(ctx, in.ID) })
	a.methods["trace.forMemory"] = method(func(ctx context.Context, in struct {
		ID    string `json:"id"`
		Limit int    `json:"limit"`
	}) (any, error) {
		return r().List(ctx, trace.Filter{MemoryID: in.ID, Limit: in.Limit})
	})
	a.methods["trace.heat"] = method(func(ctx context.Context, f trace.Filter) (any, error) { return r().Heat(ctx, f) })
	a.methods["trace.daily"] = method(func(ctx context.Context, f trace.Filter) (any, error) { return r().Daily(ctx, f) })
}

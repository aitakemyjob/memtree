package trace

import "context"

// Origin attributes a retrieval to the adapter that asked for it.
type Origin struct {
	Kind    string // one of the Origin* constants
	Tool    string
	Harness string
	Session string
}

// originKey is the context key for an Origin.
type originKey struct{}

// WithOrigin returns ctx carrying o. An inner call overrides an outer one,
// so the most specific adapter wins (a hook relayed through the API).
func WithOrigin(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, originKey{}, o)
}

// OriginFrom returns the Origin on ctx, or an internal one when none is set.
func OriginFrom(ctx context.Context) Origin {
	if o, ok := ctx.Value(originKey{}).(Origin); ok && o.Kind != "" {
		return o
	}
	return Origin{Kind: OriginInternal}
}

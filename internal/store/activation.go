package store

import (
	"math"
	"time"
)

// ActivationParams tune the ACT-R activation curve for one kind of entity.
type ActivationParams struct {
	DecayExponent    float64 // d
	Floor            float64 // minimum multiplier
	ReferenceCeiling float64 // logistic scale; larger = gentler slope
}

// Decay exponents per entity. Semantic memory decays slowest: it is already
// distilled gist and should stay available.
const (
	DecayEpisodic = 0.5
	DecaySemantic = 0.2
	DecayNode     = 0.35
	DefaultFloor  = 0.15
	AccessLogCap  = 20
)

// Activation params for each tier and for lattice nodes.
var (
	EpisodicParams = ActivationParams{DecayExponent: DecayEpisodic, Floor: DefaultFloor, ReferenceCeiling: 2}
	SemanticParams = ActivationParams{DecayExponent: DecaySemantic, Floor: DefaultFloor, ReferenceCeiling: 2}
	NodeParams     = ActivationParams{DecayExponent: DecayNode, Floor: DefaultFloor, ReferenceCeiling: 2}
)

// ParamsFor returns the activation params for a memory tier.
func ParamsFor(tier string) ActivationParams {
	if tier == TierSemantic {
		return SemanticParams
	}
	return EpisodicParams
}

// ActivationRaw is ln(Σ tᵢ^-d) where tᵢ is hours since access i (min 1h).
// An empty log is -Inf.
func ActivationRaw(log []time.Time, d float64, now time.Time) float64 {
	if len(log) == 0 {
		return math.Inf(-1)
	}
	sum := 0.0
	for _, at := range log {
		hours := math.Max(1, now.Sub(at).Hours())
		sum += math.Pow(hours, -d)
	}
	return math.Log(sum)
}

// ActivationScore squashes raw activation through a logistic centred at 0
// into (0,1), then lifts it into [floor, 1]. With no accesses it is the floor.
func ActivationScore(log []time.Time, p ActivationParams, now time.Time) float64 {
	raw := ActivationRaw(log, p.DecayExponent, now)
	if math.IsInf(raw, 0) || math.IsNaN(raw) {
		return p.Floor
	}
	scale := p.ReferenceCeiling
	if scale <= 0 {
		scale = 1
	}
	normalized := 1 / (1 + math.Exp(-raw/scale))
	return p.Floor + (1-p.Floor)*normalized
}

// TouchAccessLog appends at and keeps the most recent AccessLogCap entries.
func TouchAccessLog(log []time.Time, at time.Time) []time.Time {
	next := append(append([]time.Time(nil), log...), at)
	if len(next) > AccessLogCap {
		next = next[len(next)-AccessLogCap:]
	}
	return next
}

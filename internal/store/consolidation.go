package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// TierConfig drives when memories age into compressed form. Defaults are
// cortex's: episodic records compress after 72h, semantic after 720h, and a
// record still "hot" (activation above the ceiling) is never touched.
type TierConfig struct {
	AgeHoursToSemantic        float64 `json:"ageHoursToSemantic"`
	AgeHoursToCompact         float64 `json:"ageHoursToCompact"`
	ActivationCeilingCompress float64 `json:"activationCeilingToCompress"`
	MinClusterSize            int     `json:"minClusterSize"`
}

// DefaultTierConfig returns the ported defaults.
func DefaultTierConfig() TierConfig {
	return TierConfig{AgeHoursToSemantic: 72, AgeHoursToCompact: 720, ActivationCeilingCompress: 0.5, MinClusterSize: 2}
}

// clusterJaccard is the minimum keyword overlap for two records to cluster.
// It is deliberately a constant, not config: it matches cortex.
const clusterJaccard = 0.2

// ShouldCompress decides whether a record has aged and cooled enough to be
// distilled. Pinned records never compress. A distilled record gets a fresh
// CreatedAt, so it cannot be re-distilled until another full interval passes.
func ShouldCompress(r MemoryRecord, activation float64, now time.Time, cfg TierConfig) bool {
	if r.Pinned || r.ArchivedAt != nil || r.SupersededBy != "" {
		return false
	}
	if activation > cfg.ActivationCeilingCompress {
		return false
	}
	age := now.Sub(r.CreatedAt).Hours()
	if r.Tier == TierEpisodic {
		return age >= cfg.AgeHoursToSemantic
	}
	return age >= cfg.AgeHoursToCompact
}

// Cluster is a group of related, compressible records an agent can distil
// into one semantic record.
type Cluster struct {
	Records []MemoryRecord `json:"records"`
	Project string         `json:"project"`
	Shared  []string       `json:"sharedKeywords"`
}

// ClusterRecords greedily groups records: one joins the first cluster holding
// a record it is related to (same project and keyword Jaccard ≥ 0.2). Input
// order matters (newest first, as List returns), clusters smaller than
// minSize are dropped.
func ClusterRecords(records []MemoryRecord, minSize int) [][]MemoryRecord {
	var clusters [][]MemoryRecord
outer:
	for _, r := range records {
		for i, c := range clusters {
			for _, m := range c {
				if related(m, r) {
					clusters[i] = append(clusters[i], r)
					continue outer
				}
			}
		}
		clusters = append(clusters, []MemoryRecord{r})
	}
	var out [][]MemoryRecord
	for _, c := range clusters {
		if len(c) >= minSize {
			out = append(out, c)
		}
	}
	return out
}

// related reports whether two records may share a compression cluster.
func related(a, b MemoryRecord) bool {
	if a.Project != b.Project {
		return false
	}
	return jaccard(a.Keywords, b.Keywords) >= clusterJaccard
}

// jaccard is |A∩B| / |A∪B| over lowercased keywords. Two empty sets count as
// identical (1) here, unlike keyword-coverage matching, which treats empty as
// no overlap; the two helpers intentionally differ.
func jaccard(a, b []string) float64 {
	sa, sb := lowerSet(a), lowerSet(b)
	if len(sa) == 0 && len(sb) == 0 {
		return 1
	}
	shared := 0
	for k := range sa {
		if sb[k] {
			shared++
		}
	}
	union := len(sa) + len(sb) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

// lowerSet lowercases and trims strings into a set.
func lowerSet(in []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range in {
		m[strings.ToLower(strings.TrimSpace(s))] = true
	}
	return m
}

// Candidates finds clusters of aged, cooled records an agent should distil.
// memtree has no model, so it proposes and the agent writes the gist; see
// Compress. project scopes the sweep ("" with all=false is global only).
func (m *Memory) Candidates(ctx context.Context, cfg TierConfig, project string, all bool) ([]Cluster, error) {
	recs, err := m.List(ctx, Query{Project: project, AllProjects: all})
	if err != nil {
		return nil, err
	}
	now := m.db.Now()
	var eligible []MemoryRecord
	for _, r := range recs {
		if r.Kind == KindCapture {
			continue // raw captures await summary, not compression
		}
		if ShouldCompress(r, m.ActivationOf(r), now, cfg) {
			eligible = append(eligible, r)
		}
	}
	out := []Cluster{}
	for _, c := range ClusterRecords(eligible, cfg.MinClusterSize) {
		out = append(out, Cluster{Records: c, Project: c[0].Project, Shared: sharedKeywords(c)})
	}
	return out, nil
}

// sharedKeywords lists keywords common to every record in a cluster.
func sharedKeywords(c []MemoryRecord) []string {
	count := map[string]int{}
	var order []string
	for _, r := range c {
		for k := range lowerSet(r.Keywords) {
			if count[k] == 0 {
				order = append(order, k)
			}
			count[k]++
		}
	}
	var out []string
	for _, k := range order {
		if count[k] == len(c) {
			out = append(out, k)
		}
	}
	return nonNil(out)
}

var provRank = map[string]int{ProvInferred: 0, ProvSource: 1, ProvHuman: 2}

// Compress saves an agent-written semantic gist and supersedes the records it
// replaces, atomically. The gist cannot inflate what it came from: importance
// is the inputs' maximum, provenance their weakest, and it starts with a fresh
// access log and creation time. Originals are retained (superseded, not
// deleted) so the step is reversible. Pinned or unknown inputs are refused.
func (m *Memory) Compress(ctx context.Context, inputIDs []string, gist MemoryRecord) (MemoryRecord, error) {
	inputIDs = cleanStrings(inputIDs)
	if len(inputIDs) < 1 {
		return gist, errors.New("compress needs at least one input memory")
	}
	var inputs []MemoryRecord
	for _, id := range inputIDs {
		r, err := m.Get(ctx, id)
		if err != nil {
			return gist, err
		}
		if r.Pinned {
			return gist, errors.New("memory " + id + " is pinned and cannot be compressed")
		}
		if r.SupersededBy != "" {
			return gist, errors.New("memory " + id + " is already superseded")
		}
		inputs = append(inputs, r)
	}
	maxImp, weakest := ImportanceRoutine, ProvHuman
	paths := map[string]bool{}
	var order []string
	for _, r := range inputs {
		if r.Importance > maxImp {
			maxImp = r.Importance
		}
		if provRank[r.Provenance] < provRank[weakest] {
			weakest = r.Provenance
		}
		for _, p := range r.LatticePaths {
			if !paths[p] {
				paths[p] = true
				order = append(order, p)
			}
		}
	}
	gist.ID, gist.CreatedAt = "", time.Time{}
	gist.Tier, gist.Importance, gist.Provenance = TierSemantic, maxImp, weakest
	gist.AccessLog, gist.Pinned, gist.SupersededBy, gist.ArchivedAt = nil, false, "", nil
	gist.Project = inputs[0].Project
	if gist.Kind == "" || gist.Kind == KindCapture {
		gist.Kind = KindNote
	}
	if len(gist.LatticePaths) == 0 {
		gist.LatticePaths = order
	}
	saved, err := m.Save(ctx, gist)
	if err != nil {
		return saved, err
	}
	for _, id := range inputIDs {
		if err := m.mutate(ctx, id, func(r *MemoryRecord) { r.SupersededBy = saved.ID }); err != nil {
			return saved, err
		}
	}
	return saved, nil
}

// Lineage returns the chain of records a memory superseded (its sources),
// for the UI's "what was this distilled from" view.
func (m *Memory) Lineage(ctx context.Context, id string) ([]MemoryRecord, error) {
	rows, err := m.db.sql.QueryContext(ctx, `SELECT json FROM memory_records WHERE superseded_by = ? ORDER BY created_at DESC`, id)
	if err != nil {
		return nil, err
	}
	var out []MemoryRecord
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			rows.Close()
			return nil, err
		}
		r, err := decode(blob)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i], err = m.fillPaths(ctx, out[i]); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if out == nil {
		out = []MemoryRecord{}
	}
	return out, nil
}

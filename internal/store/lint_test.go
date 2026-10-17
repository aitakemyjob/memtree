package store

import (
	"strings"
	"testing"
)

func TestSimilarLabels(t *testing.T) {
	yes := [][2]string{{"db", "database"}, {"deploy", "deploys"}, {"api-gateway", "apigateway"}, {"terraform", "terrafrom"}, {"tool", "tools"}, {"repo", "repository"}, {"cfg", "config"}, {"env", "environment"}}
	no := [][2]string{{"api", "app"}, {"storage", "audio"}, {"cache", "cash"}, {"ci", "cd"}, {"auth", "author"}}
	for _, p := range yes {
		if ok, _ := similarLabels(p[0], p[1]); !ok {
			t.Errorf("%v should be similar", p)
		}
	}
	for _, p := range no {
		if ok, why := similarLabels(p[0], p[1]); ok {
			t.Errorf("%v should not be similar (%s)", p, why)
		}
	}
	if editDistance("kitten", "sitting") != 3 || editDistance("", "abc") != 3 || editDistance("same", "same") != 0 {
		t.Error("levenshtein")
	}
}

func TestLint(t *testing.T) {
	f := newFixture(t)
	f.lat.SeedAxes(f.ctx)
	a, _ := f.mem.Save(f.ctx, rec("uses the db layer", func(r *MemoryRecord) { r.LatticePaths = []string{"project/db"} }))
	f.mem.Save(f.ctx, rec("database migrations run on boot", func(r *MemoryRecord) { r.LatticePaths = []string{"project/database", "stray/topic"} }))
	f.mem.Save(f.ctx, rec("nothing confirmed here", nil))
	f.mem.Save(f.ctx, rec("raw capture text", func(r *MemoryRecord) { r.Kind = KindCapture }))
	rep, err := f.mem.Lint(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.NearDuplicates) != 1 || rep.NearDuplicates[0].A != "project/database" || rep.NearDuplicates[0].B != "project/db" {
		t.Errorf("near duplicates %+v", rep.NearDuplicates)
	}
	if len(rep.OutsideAxes) != 1 || rep.OutsideAxes[0] != "stray" {
		t.Errorf("outside axes %v", rep.OutsideAxes)
	}
	var single []string
	for _, u := range rep.SingleUse {
		single = append(single, u.Category)
	}
	if !strings.Contains(strings.Join(single, ","), "project/db") || strings.Contains(strings.Join(single, ","), "project,") {
		t.Errorf("single use %v", single)
	}
	if len(rep.WeaklyFiled) != 1 || !strings.Contains(rep.WeaklyFiled[0].Summary, "nothing confirmed") {
		t.Errorf("weakly filed %+v (raw captures are exempt, confirmed ones too)", rep.WeaklyFiled)
	}
	// Fixing the findings clears them.
	f.mem.Update(f.ctx, a.ID, func(*MemoryRecord) {}, []string{"project/database"})
	f.lat.PruneEmpty(f.ctx, AxisSeeds)
	rep, _ = f.mem.Lint(f.ctx)
	if len(rep.NearDuplicates) != 0 {
		t.Errorf("after retagging and pruning: %+v", rep.NearDuplicates)
	}
}

func TestCheckPaths(t *testing.T) {
	f := newFixture(t)
	f.lat.SeedAxes(f.ctx)
	f.lat.EnsureNode(f.ctx, "project/database")
	f.lat.EnsureNode(f.ctx, "knowledge/tools/redis")
	w, err := f.lat.CheckPaths(f.ctx, []string{"project/db", "project/database", "misc/stuff", "knowledge/tools/redis", "knowledge/tools/reddis"})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]PathWarning{}
	for _, x := range w {
		by[x.Path] = x
	}
	if by["project/db"].Suggest != "project/database" {
		t.Errorf("abbreviation should suggest the existing node: %+v", by["project/db"])
	}
	if !strings.Contains(by["misc/stuff"].Message, "not a standard axis") {
		t.Errorf("stray root: %+v", by["misc/stuff"])
	}
	if _, bad := by["project/database"]; bad {
		t.Error("an existing path draws no warning")
	}
	if _, bad := by["knowledge/tools/redis"]; bad {
		t.Error("an existing deep path draws no warning")
	}
	if by["knowledge/tools/reddis"].Suggest != "knowledge/tools/redis" {
		t.Errorf("deep near-duplicate: %+v", by["knowledge/tools/reddis"])
	}
}

func TestExcludeKindAndCaptureNotQueued(t *testing.T) {
	f := newFixture(t)
	f.mem.Save(f.ctx, rec("a real note", nil))
	f.mem.Save(f.ctx, rec("raw capture digest", func(r *MemoryRecord) { r.Kind = KindCapture }))
	if n, _ := f.lat.UntaggedCount(f.ctx); n != 1 {
		t.Errorf("only the real note should await classification, got %d", n)
	}
	all, _ := f.mem.List(f.ctx, Query{Project: "/repo/a"})
	hid, _ := f.mem.List(f.ctx, Query{Project: "/repo/a", ExcludeKind: KindCapture})
	if len(all) != 2 || len(hid) != 1 || hid[0].Kind == KindCapture {
		t.Errorf("exclude kind: all=%d hidden=%d", len(all), len(hid))
	}
}

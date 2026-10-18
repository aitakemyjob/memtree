package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/core"
)

func newAPI(t *testing.T) *API {
	t.Helper()
	cfg, _ := config.Open(filepath.Join(t.TempDir(), "c.json"))
	svc, err := core.Open(cfg, ":memory:", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return New(svc)
}

func call(t *testing.T, a *API, method, args string) string {
	t.Helper()
	out, err := a.Invoke(context.Background(), method, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return string(out)
}

func TestMemoryLifecycleOverAPI(t *testing.T) {
	a := newAPI(t)
	var saved struct{ ID string }
	json.Unmarshal([]byte(call(t, a, "memory.save", `{"summary":"api note about caching","project":"/p","latticePaths":["project/architecture"]}`)), &saved)
	if saved.ID == "" {
		t.Fatal("no id")
	}
	if !strings.Contains(call(t, a, "memory.get", `{"id":"`+saved.ID+`"}`), `"provenance":"HUMAN_CONFIRMED"`) {
		t.Error("memories typed in the UI default to human-confirmed")
	}
	call(t, a, "memory.update", `{"id":"`+saved.ID+`","summary":"api note about eviction","importance":2.5,"setPaths":true,"latticePaths":["knowledge/tools"]}`)
	got := call(t, a, "memory.get", `{"id":"`+saved.ID+`"}`)
	if !strings.Contains(got, "eviction") || !strings.Contains(got, `"importance":2.5`) || !strings.Contains(got, "knowledge/tools") || strings.Contains(got, "project/architecture") {
		t.Errorf("update: %s", got)
	}
	call(t, a, "memory.pin", `{"id":"`+saved.ID+`","pinned":true}`)
	if !strings.Contains(call(t, a, "memory.list", `{"project":"/p","pinnedOnly":true}`), saved.ID) {
		t.Error("pinned filter")
	}
	exp := call(t, a, "memory.explain", `{"id":"`+saved.ID+`"}`)
	for _, k := range []string{`"activation":1`, `"compressible":false`, `"links"`} {
		if !strings.Contains(exp, k) {
			t.Errorf("explain missing %s: %s", k, exp)
		}
	}
	call(t, a, "memory.archive", `{"id":"`+saved.ID+`"}`)
	if strings.Contains(call(t, a, "memory.list", `{"project":"/p"}`), saved.ID) {
		t.Error("archived should be hidden")
	}
	if !strings.Contains(call(t, a, "memory.list", `{"project":"/p","includeArchived":true}`), saved.ID) {
		t.Error("includeArchived")
	}
	call(t, a, "memory.delete", `{"id":"`+saved.ID+`"}`)
	if _, err := a.Invoke(context.Background(), "memory.get", json.RawMessage(`{"id":"`+saved.ID+`"}`)); err == nil {
		t.Error("deleted")
	}
}

func TestListsNeverSerializeAsNull(t *testing.T) {
	a := newAPI(t)
	for _, m := range []string{"memory.list", "captures.list", "captures.pending", "lattice.untagged", "consolidation.candidates", "activity.list", "projects.list"} {
		if got := call(t, a, m, `{}`); got == "null" {
			t.Errorf("%s returned null", m)
		}
	}
	if got := call(t, a, "memory.recall", `{"query":"nothing here"}`); !strings.Contains(got, `"memories":[]`) {
		t.Errorf("recall empty: %s", got)
	}
}

func TestAppStateSettingsAndErrors(t *testing.T) {
	a := newAPI(t)
	st := call(t, a, "app.state", ``)
	for _, k := range []string{`"version":"test"`, `"axisRoots"`, `"settings"`, `"stats"`} {
		if !strings.Contains(st, k) {
			t.Errorf("app.state missing %s", k)
		}
	}
	out := call(t, a, "settings.update", `{"theme":"paper","fontSize":14,"mcpPort":47480,"agentWrites":false,"captureHooks":true,"surfaceThreshold":0.3,"maxSurface":2,"linkHalfLifeDays":30,"maintainHours":12,"tiers":{"ageHoursToSemantic":72,"ageHoursToCompact":720,"activationCeilingToCompress":0.5,"minClusterSize":2}}`)
	if !strings.Contains(out, `"theme":"paper"`) || !strings.Contains(out, `"agentWrites":false`) {
		t.Errorf("settings: %s", out)
	}
	if _, err := a.Invoke(context.Background(), "settings.update", json.RawMessage(`{"mcpPort":99999}`)); err == nil {
		t.Error("invalid settings must error")
	}
	if _, err := a.Invoke(context.Background(), "memory.list", json.RawMessage(`{"before":"yesterday"}`)); err == nil {
		t.Error("bad time must error")
	}
	if _, err := a.Invoke(context.Background(), "memory.list", json.RawMessage(`{bad`)); err == nil {
		t.Error("bad json must error")
	}
	if _, err := a.Invoke(context.Background(), "hooks.status", nil); err == nil {
		t.Error("hooks unavailable without a manager")
	}
	if len(a.Methods()) < 30 {
		t.Errorf("methods: %v", a.Methods())
	}
}

func TestLatticeMethods(t *testing.T) {
	a := newAPI(t)
	var m struct{ ID string }
	json.Unmarshal([]byte(call(t, a, "memory.save", `{"summary":"terraform workspaces map to environments","project":"/p"}`)), &m)
	if !strings.Contains(call(t, a, "lattice.untagged", `{}`), m.ID) {
		t.Error("untagged")
	}
	call(t, a, "lattice.tag", `{"id":"`+m.ID+`","paths":["knowledge/tools/terraform"]}`)
	if strings.Contains(call(t, a, "lattice.untagged", `{}`), m.ID) {
		t.Error("tag clears queue")
	}
	fire := call(t, a, "lattice.fire", `{"text":"terraform"}`)
	if !strings.Contains(fire, "knowledge/tools/terraform") || !strings.Contains(fire, m.ID) {
		t.Errorf("fire: %s", fire)
	}
	if !strings.Contains(call(t, a, "lattice.nodes", `{}`), `"id":"knowledge/tools/terraform"`) {
		t.Error("nodes")
	}
	call(t, a, "lattice.ensure", `{"path":"scratch/x"}`)
	if !strings.Contains(call(t, a, "lattice.prune", `{}`), `"removed":2`) {
		t.Error("prune")
	}
}

func TestTraceMethods(t *testing.T) {
	a := newAPI(t)
	var saved struct{ ID string }
	json.Unmarshal([]byte(call(t, a, "memory.save", `{"summary":"api note about caching","project":"/p"}`)), &saved)
	var res struct{ TraceID string }
	json.Unmarshal([]byte(call(t, a, "memory.recall", `{"query":"caching","project":"/p"}`)), &res)
	if res.TraceID == "" {
		t.Fatal("recall should return a trace id")
	}
	if got := call(t, a, "trace.get", `{"id":"`+res.TraceID+`"}`); !strings.Contains(got, `"origin":"ui"`) || !strings.Contains(got, `"tool":"memory.recall"`) || !strings.Contains(got, saved.ID) {
		t.Errorf("trace.get: %s", got)
	}
	for _, m := range []string{"trace.list", "trace.heat", "trace.daily"} {
		if got := call(t, a, m, `{}`); !strings.Contains(got, saved.ID) && !strings.Contains(got, `"retrieved":1`) {
			t.Errorf("%s: %s", m, got)
		}
	}
	if got := call(t, a, "trace.forMemory", `{"id":"`+saved.ID+`"}`); !strings.Contains(got, res.TraceID) {
		t.Errorf("trace.forMemory: %s", got)
	}
}

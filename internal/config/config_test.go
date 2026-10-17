package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMissingUsesDefaults(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	g := s.Get()
	if g.MCPPort != DefaultMCPPort || !g.AgentWrites || !g.CaptureHooks || g.Tiers.MinClusterSize != 2 {
		t.Errorf("defaults %+v", g)
	}
}

func TestUpdatePersistsAndValidates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "c.json")
	s, _ := Open(p)
	if _, err := s.Update(func(x *Settings) { x.Theme = "paper"; x.AgentWrites = false }); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if g := s2.Get(); g.Theme != "paper" || g.AgentWrites {
		t.Errorf("not persisted: %+v", g)
	}
	if _, err := s.Update(func(x *Settings) { x.MCPPort = 70000 }); err == nil {
		t.Error("bad port should fail")
	}
	if _, err := s.Update(func(x *Settings) { x.Tiers.AgeHoursToCompact = 1 }); err == nil {
		t.Error("compact < semantic should fail")
	}
	if s.Get().MCPPort != DefaultMCPPort {
		t.Error("failed update must not change settings")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("MEMTREE_CONFIG", "/x/c.json")
	t.Setenv("MEMTREE_DB", "/x/m.db")
	if p, _ := DefaultPath(); p != "/x/c.json" {
		t.Error(p)
	}
	if p, _ := DefaultDBPath(); p != "/x/m.db" {
		t.Error(p)
	}
}

func TestCorruptFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte("{nope"), 0o600)
	if _, err := Open(p); err == nil {
		t.Error("corrupt config should error, not silently reset")
	}
}

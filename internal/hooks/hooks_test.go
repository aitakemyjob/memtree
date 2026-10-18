package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/core"
	"github.com/aitakemyjob/memtree/internal/store"
	"github.com/aitakemyjob/memtree/internal/trace"
)

func newMgr(t *testing.T) (*Manager, *core.Service, string) {
	t.Helper()
	cfg, _ := config.Open(filepath.Join(t.TempDir(), "c.json"))
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	svc, err := core.Open(cfg, ":memory:", "test", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	repo, _ = filepath.EvalSymlinks(repo)
	m := NewManager(svc)
	m.Home = t.TempDir()
	m.Exe = "/opt/memtree/memtree"
	m.ClaudeCLI = func(args ...string) error { return nil }
	return m, svc, repo
}

func handle(t *testing.T, m *Manager, harness, event string, payload map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(payload)
	out, err := m.Handle(context.Background(), harness, event, raw)
	if err != nil {
		t.Fatalf("%s %s: %v", harness, event, err)
	}
	return out
}

func ctxOf(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		H struct {
			Name string `json:"hookEventName"`
			Ctx  string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %q", out)
	}
	return v.H.Ctx
}

func TestClaudeCodeFullFlow(t *testing.T) {
	m, svc, repo := newMgr(t)
	ctx := context.Background()
	svc.Save(ctx, store.MemoryRecord{Summary: "websocket reconnect uses exponential backoff", Project: repo, LatticePaths: []string{"project/architecture/realtime"}, OpenLoops: []string{"add jitter"}})

	start := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "SessionStart", "session_id": "s1", "cwd": repo, "source": "startup"})
	if c := ctxOf(t, start); !strings.Contains(c, "memtree is connected") || !strings.Contains(c, "add jitter") {
		t.Errorf("anchor: %q", c)
	}
	var env map[string]map[string]any
	json.Unmarshal([]byte(start), &env)
	if env["hookSpecificOutput"]["hookEventName"] != "SessionStart" {
		t.Error("output must echo the event name")
	}

	p := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": repo, "prompt": "why does the websocket reconnect lag?"})
	if c := ctxOf(t, p); !strings.Contains(c, "exponential backoff") || !strings.Contains(c, "may be outdated") {
		t.Errorf("surface: %q", c)
	}
	if q := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": repo, "prompt": "thanks, ok"}); q != "" {
		t.Errorf("off-topic prompt must print nothing, got %q", q)
	}

	if stop := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "Stop", "session_id": "s1", "cwd": repo, "last_assistant_message": "Added jitter to the backoff."}); stop != "{}" {
		t.Errorf("stop: %q", stop)
	}
	turns, _ := svc.Capture.Turns(ctx, "s1")
	if len(turns) != 3 || turns[0].Role != "user" || turns[2].Role != "assistant" {
		t.Errorf("turns %+v", turns)
	}
	pend, _ := svc.Capture.Pending(ctx, repo, false, 0)
	if len(pend) != 1 {
		t.Errorf("a checkpoint should leave a pending capture: %+v", pend)
	}
	// Re-entrant stop must not duplicate the capture.
	handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "Stop", "session_id": "s1", "cwd": repo, "last_assistant_message": "again", "stop_hook_active": true})
	if turns, _ = svc.Capture.Turns(ctx, "s1"); len(turns) != 3 {
		t.Error("stop_hook_active must not capture")
	}
	handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "SessionEnd", "session_id": "s1", "cwd": repo, "reason": "prompt_input_exit"})
	s, _ := svc.Capture.Session(ctx, "s1")
	if s.CloseReason != "prompt_input_exit" || s.ClosedAt == nil {
		t.Errorf("session %+v", s)
	}
	// A new session sees the previous one awaiting a summary.
	next := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "SessionStart", "session_id": "s2", "cwd": repo})
	if !strings.Contains(ctxOf(t, next), "1 recent session(s)") {
		t.Errorf("next anchor: %q", ctxOf(t, next))
	}
	// …but a resumed session does not count itself.
	again := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "SessionStart", "session_id": "s1", "cwd": repo, "source": "resume"})
	if strings.Contains(ctxOf(t, again), "recent session(s)") {
		t.Error("resumed session counted itself as pending")
	}
}

func TestCodexAndVSCodeShareContract(t *testing.T) {
	m, _, repo := newMgr(t)
	for _, h := range []string{Codex, VSCode} {
		out := handle(t, m, h, "SessionStart", map[string]any{"session_id": "x-" + h, "cwd": repo})
		if !strings.Contains(ctxOf(t, out), "memtree is connected") {
			t.Errorf("%s: %q", h, out)
		}
		if s := handle(t, m, h, "Stop", map[string]any{"session_id": "x-" + h, "cwd": repo, "last_assistant_message": "ok"}); s != "{}" {
			t.Errorf("%s stop: %q", h, s)
		}
	}
}

func TestCursorContract(t *testing.T) {
	m, svc, repo := newMgr(t)
	ctx := context.Background()
	svc.Save(ctx, store.MemoryRecord{Summary: "cursor rules live in .cursor/rules", Project: repo, Pinned: true})
	start := handle(t, m, Cursor, "sessionStart", map[string]any{"conversation_id": "c1", "workspace_roots": []string{repo}, "hook_event_name": "sessionStart"})
	var v map[string]string
	if err := json.Unmarshal([]byte(start), &v); err != nil || !strings.Contains(v["additional_context"], "memtree is connected") || !strings.Contains(v["additional_context"], "cursor rules") {
		t.Errorf("sessionStart: %q", start)
	}
	if bp := handle(t, m, Cursor, "beforeSubmitPrompt", map[string]any{"conversation_id": "c1", "workspace_roots": []string{repo}, "prompt": "refactor the parser"}); bp != `{"continue":true}` {
		t.Errorf("beforeSubmitPrompt must only continue: %q", bp)
	}
	handle(t, m, Cursor, "afterAgentResponse", map[string]any{"conversation_id": "c1", "workspace_roots": []string{repo}, "text": "Parser refactored into three passes."})
	turns, _ := svc.Capture.Turns(ctx, "c1")
	if len(turns) != 2 || turns[1].Text != "Parser refactored into three passes." {
		t.Errorf("turns %+v", turns)
	}
	s, _ := svc.Capture.Session(ctx, "c1")
	if s.Project != repo {
		t.Errorf("project from workspace_roots: %q", s.Project)
	}
}

func TestFailuresAreReportedNotFatal(t *testing.T) {
	m, svc, _ := newMgr(t)
	if _, err := m.Handle(context.Background(), "nope", "x", []byte(`{}`)); err == nil {
		t.Error("unknown harness errors")
	}
	if _, err := m.Handle(context.Background(), ClaudeCode, "", []byte(`{bad`)); err == nil {
		t.Error("bad JSON errors")
	}
	if out, err := m.Handle(context.Background(), ClaudeCode, "Notification", []byte(`{"session_id":"z"}`)); err != nil || out != "" {
		t.Errorf("unhandled events are ignored: %q %v", out, err)
	}
	var errs int
	for _, a := range svc.ActivityLog() {
		if a.Error != "" {
			errs++
		}
		if a.Source != "hook" {
			t.Errorf("source %q", a.Source)
		}
	}
	if errs != 2 {
		t.Errorf("both failures should be in the activity feed, got %d", errs)
	}
}

func TestCaptureOffStillRecalls(t *testing.T) {
	m, svc, repo := newMgr(t)
	ctx := context.Background()
	svc.Save(ctx, store.MemoryRecord{Summary: "graphql schema is generated from sdl", Project: repo, LatticePaths: []string{"knowledge/tools/graphql"}})
	svc.UpdateSettings(func(s *config.Settings) { s.CaptureHooks = false })
	out := handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "q", "cwd": repo, "prompt": "how is the graphql schema built"})
	if !strings.Contains(ctxOf(t, out), "generated from sdl") {
		t.Errorf("recall must still work: %q", out)
	}
	if _, err := svc.Capture.Session(ctx, "q"); err == nil {
		t.Error("capture off must record nothing")
	}
}

func TestInjectionIsCapped(t *testing.T) {
	long := strings.Repeat("é", maxInject*2)
	out := render(ClaudeCode, "SessionStart", long)
	if got := len([]rune(ctxOf(t, out))); got > maxInject {
		t.Errorf("injected %d runes", got)
	}
}

// ---- installer ----

func readDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return d
}

func TestInstallClaudeCodeMergesAndIsIdempotent(t *testing.T) {
	m, _, _ := newMgr(t)
	settings := filepath.Join(m.Home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte(`{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]}]}}`), 0o644)
	var cli [][]string
	m.ClaudeCLI = func(a ...string) error { cli = append(cli, a); return nil }

	dry, _ := m.Install(context.Background(), ClaudeCode, "", true)
	if !dry.(*Result).DryRun || readDoc(t, settings)["hooks"].(map[string]any)["SessionStart"] != nil {
		t.Error("dry run must not write")
	}
	for i := 0; i < 2; i++ {
		if _, err := m.Install(context.Background(), ClaudeCode, "", false); err != nil {
			t.Fatal(err)
		}
	}
	doc := readDoc(t, settings)
	if doc["model"] != "opus" {
		t.Error("unrelated settings lost")
	}
	hooks := doc["hooks"].(map[string]any)
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Stop", "PreCompact", "SessionEnd"} {
		if n := len(hooks[ev].([]any)); (ev == "Stop" && n != 2) || (ev != "Stop" && n != 1) {
			t.Errorf("%s has %d entries (idempotency / preserve others)", ev, n)
		}
	}
	stop := hooks["Stop"].([]any)
	if stop[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] != "notify-send done" {
		t.Error("the user's own Stop hook must survive and stay first")
	}
	cmd := hooks["SessionStart"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if cmd != "/opt/memtree/memtree hook claude-code" {
		t.Errorf("command %q", cmd)
	}
	if _, err := os.Stat(settings + ".memtree.bak"); err != nil {
		t.Error("backup missing")
	}
	var adds int
	for _, c := range cli {
		if c[0] == "mcp" && c[1] == "add" {
			adds++
			if c[len(c)-2] != "/opt/memtree/memtree" || c[len(c)-1] != "mcp" {
				t.Errorf("mcp add args %v", c)
			}
		}
	}
	if adds != 2 {
		t.Errorf("mcp add ran %d times", adds)
	}

	if _, err := m.Uninstall(context.Background(), ClaudeCode, ""); err != nil {
		t.Fatal(err)
	}
	doc = readDoc(t, settings)
	hooks = doc["hooks"].(map[string]any)
	if len(hooks) != 1 || len(hooks["Stop"].([]any)) != 1 || doc["model"] != "opus" {
		t.Errorf("uninstall must remove only memtree: %v", doc)
	}
}

func TestInstallRefusesNonJSON(t *testing.T) {
	m, _, repo := newMgr(t)
	os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	os.WriteFile(filepath.Join(repo, ".claude", "settings.json"), []byte("{\n // a comment\n}"), 0o644)
	if _, err := m.Install(context.Background(), ClaudeCode, repo, false); err == nil || !strings.Contains(err.Error(), "will not overwrite") {
		t.Errorf("want refusal, got %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".claude", "settings.json"))
	if !strings.Contains(string(b), "a comment") {
		t.Error("file must be untouched")
	}
}

func TestInstallCodexTOMLAndHooks(t *testing.T) {
	m, _, _ := newMgr(t)
	toml := filepath.Join(m.Home, ".codex", "config.toml")
	os.MkdirAll(filepath.Dir(toml), 0o755)
	orig := "model = \"gpt-5\"\n\n[mcp_servers.other]\ncommand = \"x\"\n"
	os.WriteFile(toml, []byte(orig), 0o644)
	for i := 0; i < 2; i++ {
		if _, err := m.Install(context.Background(), Codex, "", false); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(toml)
	s := string(b)
	if strings.Count(s, "[mcp_servers.memtree]") != 1 || !strings.HasPrefix(s, orig) || !strings.Contains(s, `command = "/opt/memtree/memtree"`) {
		t.Errorf("toml:\n%s", s)
	}
	if h := readDoc(t, filepath.Join(m.Home, ".codex", "hooks.json"))["hooks"].(map[string]any); h["UserPromptSubmit"] == nil {
		t.Error("codex hooks missing")
	}
	m.Uninstall(context.Background(), Codex, "")
	b, _ = os.ReadFile(toml)
	if string(b) != orig {
		t.Errorf("uninstall should restore the original text:\n%s", b)
	}
	// An unmanaged definition is never overwritten.
	os.WriteFile(toml, []byte("[mcp_servers.memtree]\ncommand = \"mine\"\n"), 0o644)
	if _, err := m.Install(context.Background(), Codex, "", false); err == nil {
		t.Error("must not clobber a hand-written memtree entry")
	}
}

func TestInstallCursorAndVSCode(t *testing.T) {
	m, _, repo := newMgr(t)
	if _, err := m.Install(context.Background(), Cursor, "", false); err != nil {
		t.Fatal(err)
	}
	hk := readDoc(t, filepath.Join(m.Home, ".cursor", "hooks.json"))
	if hk["version"] != float64(1) {
		t.Error("cursor hooks need version 1")
	}
	entry := hk["hooks"].(map[string]any)["sessionStart"].([]any)[0].(map[string]any)
	if entry["command"] != "/opt/memtree/memtree hook cursor sessionStart" {
		t.Errorf("cursor command %v", entry["command"])
	}
	mcp := readDoc(t, filepath.Join(m.Home, ".cursor", "mcp.json"))["mcpServers"].(map[string]any)["memtree"].(map[string]any)
	if mcp["command"] != "/opt/memtree/memtree" {
		t.Errorf("cursor mcp %v", mcp)
	}

	if _, err := m.Install(context.Background(), VSCode, "", false); err == nil {
		t.Error("vscode needs a project")
	}
	if _, err := m.Install(context.Background(), VSCode, repo, false); err != nil {
		t.Fatal(err)
	}
	if v := readDoc(t, filepath.Join(repo, ".vscode", "mcp.json"))["servers"].(map[string]any)["memtree"].(map[string]any); v["type"] != "stdio" {
		t.Errorf("vscode mcp %v", v)
	}
	m.Uninstall(context.Background(), VSCode, repo)
	if _, err := os.Stat(filepath.Join(repo, ".github", "hooks", "memtree.json")); err == nil {
		t.Error("vscode hooks file should be deleted on uninstall")
	}
}

func TestStatus(t *testing.T) {
	m, _, _ := newMgr(t)
	m.Install(context.Background(), Cursor, "", false)
	out, _ := m.Status(context.Background())
	st := out.(Status)
	if len(st.Harnesses) != 4 {
		t.Fatal(st)
	}
	for _, h := range st.Harnesses {
		want := h.Harness == Cursor
		if h.Hooks != want || h.MCP != want {
			t.Errorf("%s hooks=%v mcp=%v", h.Harness, h.Hooks, h.MCP)
		}
	}
}

func TestOurs(t *testing.T) {
	for cmd, want := range map[string]bool{
		"/opt/memtree/memtree hook claude-code":                   true,
		`"C:\Program Files\memtree\memtree.exe" hook claude-code`: true,
		"notify-send hook claude-code":                            false,
		"/opt/memtree/memtree hook cursor sessionStart":           false,
	} {
		if got := ours(cmd, ClaudeCode); got != want {
			t.Errorf("%q: %v want %v", cmd, got, want)
		}
	}
}

func TestHookSurfaceThenCitationIsTracedAsUse(t *testing.T) {
	m, svc, repo := newMgr(t)
	ctx := context.Background()
	rec, _ := svc.Save(ctx, store.MemoryRecord{Summary: "websocket reconnect uses exponential backoff", Project: repo, LatticePaths: []string{"project/architecture/realtime"}})
	handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": repo, "prompt": "why does the websocket reconnect lag?"})
	handle(t, m, ClaudeCode, "", map[string]any{"hook_event_name": "Stop", "session_id": "s1", "cwd": repo, "last_assistant_message": "As " + rec.ID + " notes, backoff is exponential."})
	es, _ := svc.Traces.List(ctx, trace.Filter{MemoryID: rec.ID})
	if len(es) != 1 {
		t.Fatalf("want the surfacing traced, got %d", len(es))
	}
	e := es[0]
	if e.Origin != trace.OriginHook || e.Tool != "UserPromptSubmit" || e.Harness != ClaudeCode || e.SessionID != "s1" {
		t.Errorf("event: %+v", e)
	}
	if len(e.Uses) != 1 || e.Uses[0].Evidence != trace.EvidenceCited {
		t.Errorf("citing the id should count as use: %+v", e.Uses)
	}
}

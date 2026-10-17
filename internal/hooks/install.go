package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Change describes one edit an install or uninstall made (or would make).
type Change struct {
	File   string `json:"file"`
	Action string `json:"action"` // add | update | remove | run | note
	Detail string `json:"detail"`
}

// Result is the outcome of an install or uninstall.
type Result struct {
	Harness string   `json:"harness"`
	DryRun  bool     `json:"dryRun"`
	Changes []Change `json:"changes"`
	Notes   []string `json:"notes"`
}

// HarnessStatus reports what is installed for one harness.
type HarnessStatus struct {
	Harness   string   `json:"harness"`
	Name      string   `json:"name"`
	Detected  bool     `json:"detected"` // the harness's config directory or CLI exists
	Hooks     bool     `json:"hooks"`
	MCP       bool     `json:"mcp"`
	Files     []string `json:"files"`
	Limits    string   `json:"limits,omitempty"`
	Supported []string `json:"events"`
}

// Status is the UI's view of every harness.
type Status struct {
	Exe       string          `json:"exe"`
	Harnesses []HarnessStatus `json:"harnesses"`
}

var harnessNames = map[string]string{
	ClaudeCode: "Claude Code", Codex: "Codex CLI", Cursor: "Cursor", VSCode: "VS Code (Copilot agent)",
}

var harnessLimits = map[string]string{
	ClaudeCode: "Full: session anchor, per-prompt recall, capture, checkpoints.",
	Codex:      "Full: session anchor, per-prompt recall, capture. Trust the hook once with /hooks in Codex.",
	Cursor:     "Session-start anchor and capture only: Cursor hooks cannot inject context per prompt, so rely on the recall tool.",
	VSCode:     "Preview hooks. Per-prompt injection and Stop payloads are less documented; verify with the Activity view.",
}

// events each harness is wired for, with the hook timeout in seconds.
type wiring struct {
	event   string
	timeout int
	arg     bool // pass the event name as an argument (Cursor)
}

// wiringFor lists the events memtree hooks for a harness.
func wiringFor(harness string) []wiring {
	switch harness {
	case Cursor:
		return []wiring{{"sessionStart", 10, true}, {"beforeSubmitPrompt", 10, true}, {"afterAgentResponse", 10, true}, {"preCompact", 10, true}, {"sessionEnd", 5, true}}
	case VSCode:
		return []wiring{{"SessionStart", 10, false}, {"UserPromptSubmit", 10, false}, {"Stop", 10, false}, {"PreCompact", 10, false}}
	default: // claude-code, codex
		return []wiring{{"SessionStart", 10, false}, {"UserPromptSubmit", 20, false}, {"Stop", 10, false}, {"PreCompact", 10, false}, {"SessionEnd", 5, false}}
	}
}

// exe is the executable hooks should run: Exe if set, else this binary with symlinks resolved.
func (m *Manager) exe() string {
	if m.Exe != "" {
		return m.Exe
	}
	if p, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return real
		}
		return p
	}
	return "memtree"
}

// home is the home directory, overridable for tests.
func (m *Manager) home() string {
	if m.Home != "" {
		return m.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

// command is the hook command line for a harness event.
func (m *Manager) command(harness string, w wiring) string {
	c := quote(m.exe()) + " hook " + harness
	if w.arg {
		c += " " + w.event
	}
	return c
}

// quote quotes a path containing spaces for a shell command line.
func quote(p string) string {
	if strings.ContainsAny(p, " \t()") {
		return `"` + p + `"`
	}
	return p
}

// ours reports whether a command string is a memtree hook for harness.
func ours(cmd, harness string) bool {
	return strings.Contains(strings.ToLower(cmd), "memtree") && strings.Contains(cmd, " hook "+harness)
}

// paths are the files a harness's integration lives in.
type paths struct {
	hooks    string // JSON hooks file
	mcp      string // MCP config (JSON or TOML)
	mcpTOML  bool
	ownsHook bool // memtree owns the whole hooks file (delete on uninstall)
}

// paths locates a harness's hook and MCP config files for user scope (project "") or a project.
func (m *Manager) paths(harness, project string) (paths, error) {
	h := m.home()
	switch harness {
	case ClaudeCode:
		if project != "" {
			return paths{hooks: filepath.Join(project, ".claude", "settings.json"), mcp: filepath.Join(project, ".mcp.json")}, nil
		}
		return paths{hooks: filepath.Join(h, ".claude", "settings.json")}, nil // MCP via the claude CLI
	case Codex:
		return paths{hooks: filepath.Join(h, ".codex", "hooks.json"), mcp: filepath.Join(h, ".codex", "config.toml"), mcpTOML: true}, nil
	case Cursor:
		if project != "" {
			return paths{hooks: filepath.Join(project, ".cursor", "hooks.json"), mcp: filepath.Join(project, ".cursor", "mcp.json")}, nil
		}
		return paths{hooks: filepath.Join(h, ".cursor", "hooks.json"), mcp: filepath.Join(h, ".cursor", "mcp.json")}, nil
	case VSCode:
		if project == "" {
			return paths{}, errors.New("VS Code hooks and MCP config are per workspace: pass a project directory (memtree install vscode --project .)")
		}
		return paths{hooks: filepath.Join(project, ".github", "hooks", "memtree.json"), mcp: filepath.Join(project, ".vscode", "mcp.json"), ownsHook: true}, nil
	}
	return paths{}, fmt.Errorf("unknown harness %q (want one of %s)", harness, strings.Join(Harnesses, ", "))
}

// Install wires hooks and the MCP server into a harness. It merges into
// existing files, never clobbers unrelated settings, saves a one-time backup
// (<file>.memtree.bak) before the first edit, and is idempotent. project ""
// means user scope. With dryRun it only reports what it would do.
func (m *Manager) Install(ctx context.Context, harness, project string, dryRun bool) (any, error) {
	p, err := m.paths(harness, project)
	if err != nil {
		return nil, err
	}
	res := &Result{Harness: harness, DryRun: dryRun, Changes: []Change{}, Notes: []string{}}
	if err := m.installHooks(harness, p, dryRun, res); err != nil {
		return res, err
	}
	if err := m.installMCP(ctx, harness, project, p, dryRun, res); err != nil {
		return res, err
	}
	switch harness {
	case Codex:
		res.Notes = append(res.Notes, "Codex asks you to review new hooks: run /hooks in Codex and trust the memtree entries.")
	case Cursor:
		res.Notes = append(res.Notes, harnessLimits[Cursor])
	case VSCode:
		res.Notes = append(res.Notes, harnessLimits[VSCode])
	}
	res.Notes = append(res.Notes, "Restart the harness so it picks up the change.")
	return res, nil
}

// Uninstall removes memtree's hooks and MCP entry, leaving everything else.
func (m *Manager) Uninstall(ctx context.Context, harness, project string) (any, error) {
	p, err := m.paths(harness, project)
	if err != nil {
		return nil, err
	}
	res := &Result{Harness: harness, Changes: []Change{}, Notes: []string{}}
	if p.hooks != "" {
		if p.ownsHook {
			if _, err := os.Stat(p.hooks); err == nil {
				if err := os.Remove(p.hooks); err != nil {
					return res, err
				}
				res.Changes = append(res.Changes, Change{p.hooks, "remove", "deleted memtree's hooks file"})
			}
		} else if doc, ok, err := readJSON(p.hooks); err != nil {
			return res, err
		} else if ok && stripHooks(doc, harness) {
			if err := writeJSON(p.hooks, doc, false); err != nil {
				return res, err
			}
			res.Changes = append(res.Changes, Change{p.hooks, "remove", "removed memtree hooks"})
		}
	}
	if harness == ClaudeCode && project == "" {
		if err := m.claude("mcp", "remove", "--scope", "user", "memtree"); err == nil {
			res.Changes = append(res.Changes, Change{"claude mcp", "run", "removed the memtree MCP server"})
		}
	} else if p.mcp != "" {
		if p.mcpTOML {
			if changed, err := removeTOMLBlock(p.mcp); err != nil {
				return res, err
			} else if changed {
				res.Changes = append(res.Changes, Change{p.mcp, "remove", "removed [mcp_servers.memtree]"})
			}
		} else if doc, ok, err := readJSON(p.mcp); err != nil {
			return res, err
		} else if ok {
			key := mcpKey(harness)
			if servers, _ := doc[key].(map[string]any); servers != nil {
				if _, has := servers["memtree"]; has {
					delete(servers, "memtree")
					if err := writeJSON(p.mcp, doc, false); err != nil {
						return res, err
					}
					res.Changes = append(res.Changes, Change{p.mcp, "remove", "removed the memtree MCP server"})
				}
			}
		}
	}
	return res, nil
}

// mcpKey is the top-level key holding MCP servers in a harness's JSON config.
func mcpKey(harness string) string {
	if harness == VSCode {
		return "servers"
	}
	return "mcpServers"
}

// installHooks merges memtree's hook entries into the harness hooks file.
func (m *Manager) installHooks(harness string, p paths, dryRun bool, res *Result) error {
	if p.hooks == "" {
		return nil
	}
	doc, _, err := readJSON(p.hooks)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	existed := fileExists(p.hooks)
	stripHooks(doc, harness)
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		doc["hooks"] = hooks
	}
	if harness == Cursor {
		doc["version"] = 1
	}
	var added []string
	for _, w := range wiringFor(harness) {
		entry := m.hookEntry(harness, w)
		list, _ := hooks[w.event].([]any)
		hooks[w.event] = append(list, entry)
		added = append(added, w.event)
	}
	action := "add"
	if existed {
		action = "update"
	}
	res.Changes = append(res.Changes, Change{p.hooks, action, "hooks: " + strings.Join(added, ", ")})
	if dryRun {
		return nil
	}
	return writeJSON(p.hooks, doc, true)
}

// hookEntry builds the harness-specific hook registration.
func (m *Manager) hookEntry(harness string, w wiring) any {
	cmd := m.command(harness, w)
	switch harness {
	case Cursor:
		return map[string]any{"command": cmd, "timeout": w.timeout}
	case VSCode:
		return map[string]any{"type": "command", "command": cmd, "timeout": w.timeout}
	default: // claude-code, codex: matcher group wrapping handlers
		return map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd, "timeout": w.timeout}}}
	}
}

// stripHooks removes memtree's entries from a hooks document, dropping events
// left empty. Reports whether anything was removed.
func stripHooks(doc map[string]any, harness string) bool {
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	changed := false
	for event, v := range hooks {
		list, _ := v.([]any)
		var keep []any
		for _, item := range list {
			if entryIsOurs(item, harness) {
				changed = true
				continue
			}
			keep = append(keep, item)
		}
		if len(keep) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keep
		}
	}
	if len(hooks) == 0 {
		delete(doc, "hooks")
	}
	return changed
}

// entryIsOurs reports whether a hooks-file entry (flat or grouped) is memtree's.
func entryIsOurs(item any, harness string) bool {
	e, _ := item.(map[string]any)
	if e == nil {
		return false
	}
	if cmd, _ := e["command"].(string); ours(cmd, harness) {
		return true
	}
	inner, _ := e["hooks"].([]any)
	if len(inner) == 0 {
		return false
	}
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		if cmd, _ := hm["command"].(string); !ours(cmd, harness) {
			return false // a group mixing our hook with someone else's is left alone
		}
	}
	return true
}

// installMCP registers memtree's stdio MCP bridge with the harness.
func (m *Manager) installMCP(ctx context.Context, harness, project string, p paths, dryRun bool, res *Result) error {
	exe := m.exe()
	if harness == ClaudeCode && project == "" {
		args := []string{"mcp", "add", "--scope", "user", "memtree", "--", exe, "mcp"}
		res.Changes = append(res.Changes, Change{"claude " + strings.Join(args, " "), "run", "register the MCP server (stdio bridge)"})
		if dryRun {
			return nil
		}
		_ = m.claude("mcp", "remove", "--scope", "user", "memtree") // re-adding is idempotent
		if err := m.claude(args...); err != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("Could not run the claude CLI (%v). Register the MCP server yourself:\n  claude mcp add --scope user memtree -- %s mcp", err, quote(exe)))
		}
		return nil
	}
	if p.mcp == "" {
		return nil
	}
	if p.mcpTOML {
		block := tomlBlock(exe)
		res.Changes = append(res.Changes, Change{p.mcp, "add", "[mcp_servers.memtree] → " + quote(exe) + " mcp"})
		if dryRun {
			return nil
		}
		return upsertTOMLBlock(p.mcp, block)
	}
	doc, _, err := readJSON(p.mcp)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	key := mcpKey(harness)
	servers, _ := doc[key].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		doc[key] = servers
	}
	entry := map[string]any{"command": exe, "args": []any{"mcp"}}
	if harness == VSCode || (harness == ClaudeCode) {
		entry["type"] = "stdio"
	}
	servers["memtree"] = entry
	res.Changes = append(res.Changes, Change{p.mcp, "add", "MCP server memtree → " + quote(exe) + " mcp"})
	if dryRun {
		return nil
	}
	return writeJSON(p.mcp, doc, true)
}

// claude runs the claude CLI (or the test stub).
func (m *Manager) claude(args ...string) error {
	if m.ClaudeCLI != nil {
		return m.ClaudeCLI(args...)
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		return err
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Status reports what is installed, for the UI. project "" checks user scope.
func (m *Manager) Status(ctx context.Context) (any, error) {
	st := Status{Exe: m.exe(), Harnesses: []HarnessStatus{}}
	for _, h := range Harnesses {
		hs := HarnessStatus{Harness: h, Name: harnessNames[h], Limits: harnessLimits[h], Files: []string{}}
		for _, w := range wiringFor(h) {
			hs.Supported = append(hs.Supported, w.event)
		}
		p, err := m.paths(h, "")
		if err == nil {
			hs.Detected = dirExists(filepath.Dir(p.hooks))
			if doc, ok, err := readJSON(p.hooks); err == nil && ok {
				hs.Hooks = hasOurs(doc, h)
				if hs.Hooks {
					hs.Files = append(hs.Files, p.hooks)
				}
			}
			if p.mcp != "" {
				hs.MCP = mcpPresent(h, p)
				if hs.MCP {
					hs.Files = append(hs.Files, p.mcp)
				}
			}
		}
		if h == ClaudeCode {
			if _, err := exec.LookPath("claude"); err == nil {
				hs.Detected = true
			}
		}
		if h == VSCode {
			hs.Detected = true // configured per workspace
		}
		sort.Strings(hs.Files)
		st.Harnesses = append(st.Harnesses, hs)
	}
	return st, nil
}

// hasOurs reports whether a hooks document contains any memtree entry.
func hasOurs(doc map[string]any, harness string) bool {
	hooks, _ := doc["hooks"].(map[string]any)
	for _, v := range hooks {
		list, _ := v.([]any)
		for _, item := range list {
			if entryIsOurs(item, harness) {
				return true
			}
		}
	}
	return false
}

// mcpPresent reports whether the harness's MCP config lists memtree.
func mcpPresent(harness string, p paths) bool {
	if p.mcpTOML {
		b, err := os.ReadFile(p.mcp)
		return err == nil && strings.Contains(string(b), "[mcp_servers.memtree]")
	}
	doc, ok, err := readJSON(p.mcp)
	if err != nil || !ok {
		return false
	}
	servers, _ := doc[mcpKey(harness)].(map[string]any)
	_, has := servers["memtree"]
	return has
}

// ---- file helpers ----

// fileExists reports whether p is an existing regular file.
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// dirExists reports whether p is an existing directory.
func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// readJSON reads a JSON object file. ok is false when the file is missing or
// empty. A file that is not a valid JSON object (e.g. it has comments) is an
// error: memtree refuses to rewrite what it cannot round-trip.
func readJSON(path string) (map[string]any, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, false, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, false, fmt.Errorf("%s is not plain JSON (%v); memtree will not overwrite it. Add the memtree entries by hand (see docs/MCP.md)", path, err)
	}
	return doc, true, nil
}

// writeJSON writes doc atomically. With backup it first copies an existing
// file to <path>.memtree.bak, once. Note: keys are rewritten in sorted order.
func writeJSON(path string, doc map[string]any, backup bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if backup && fileExists(path) && !fileExists(path+".memtree.bak") {
		if b, err := os.ReadFile(path); err == nil {
			if err := os.WriteFile(path+".memtree.bak", b, 0o600); err != nil {
				return err
			}
		}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

const (
	tomlBegin = "# >>> memtree (managed) >>>"
	tomlEnd   = "# <<< memtree (managed) <<<"
)

// tomlBlock renders memtree's managed [mcp_servers.memtree] block.
func tomlBlock(exe string) string {
	return tomlBegin + "\n[mcp_servers.memtree]\ncommand = " + tomlString(exe) + "\nargs = [\"mcp\"]\n" + tomlEnd + "\n"
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// upsertTOMLBlock appends memtree's managed block to a TOML file, replacing a
// previous one. Text-level on purpose: it never reparses (and so never
// reformats) the rest of the user's config.
func upsertTOMLBlock(path, block string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cur := string(b)
	if strings.Contains(cur, "[mcp_servers.memtree]") && !strings.Contains(cur, tomlBegin) {
		return fmt.Errorf("%s already defines [mcp_servers.memtree] outside memtree's managed block; edit it by hand", path)
	}
	cur = cutBlock(cur)
	if cur != "" && !strings.HasSuffix(cur, "\n") {
		cur += "\n"
	}
	if cur != "" {
		cur += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if fileExists(path) && !fileExists(path+".memtree.bak") {
		_ = os.WriteFile(path+".memtree.bak", b, 0o600)
	}
	return os.WriteFile(path, []byte(cur+block), 0o644)
}

// removeTOMLBlock deletes memtree's managed block from a TOML file, reporting whether it changed.
func removeTOMLBlock(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	next := cutBlock(string(b))
	if next == string(b) {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(next), 0o644)
}

// cutBlock removes the managed block (and one blank line before it).
func cutBlock(s string) string {
	i := strings.Index(s, tomlBegin)
	if i < 0 {
		return s
	}
	j := strings.Index(s[i:], tomlEnd)
	if j < 0 {
		return s
	}
	end := i + j + len(tomlEnd)
	if end < len(s) && s[end] == '\n' {
		end++
	}
	head := strings.TrimRight(s[:i], "\n")
	if head != "" {
		head += "\n"
	}
	return head + s[end:]
}

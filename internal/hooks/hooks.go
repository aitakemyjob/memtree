// Package hooks connects agent harnesses to memtree. A harness runs
// `memtree hook <harness>` at lifecycle events and passes a JSON payload on
// stdin; Handle turns that into memory operations and returns the text the
// harness should inject into the agent's context. Hooks never block the
// agent: any failure is logged to the activity feed and swallowed upstream.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aitakemyjob/memtree/internal/core"
)

// Supported harness names.
const (
	ClaudeCode = "claude-code"
	Codex      = "codex"
	Cursor     = "cursor"
	VSCode     = "vscode"
)

// Harnesses lists every supported harness.
var Harnesses = []string{ClaudeCode, Codex, Cursor, VSCode}

// kind is the harness-independent meaning of a lifecycle event.
type kind int

const (
	kindIgnore    kind = iota
	kindStart          // a session began (or resumed/compacted)
	kindPrompt         // the user submitted a prompt
	kindAssistant      // the agent finished a turn
	kindCompact        // context is about to be compacted
	kindEnd            // the session ended
)

// event is a normalised hook payload.
type event struct {
	Kind      kind
	Name      string // the harness's own event name
	Session   string
	Cwd       string
	Prompt    string // kindPrompt
	Text      string // kindAssistant: the agent's last message
	Reason    string // kindEnd
	Recursive bool   // the harness is re-entering a Stop hook; don't capture again
}

// maxInject caps injected context. Claude Code accepts 10,000 characters per
// string and Codex about 2,500 tokens; stay under both.
const maxInject = 8000

// Manager runs hooks and edits harness configuration.
type Manager struct {
	core *core.Service
	// Exe is the path hooks should invoke; defaults to this executable.
	Exe string
	// Home overrides the user's home directory (tests).
	Home string
	// ClaudeCLI runs `claude ...`; nil means exec the real CLI. Tests stub it.
	ClaudeCLI func(args ...string) error
}

// NewManager returns a manager over the service.
func NewManager(c *core.Service) *Manager { return &Manager{core: c} }

// Handle processes one hook event and returns the text to print on stdout
// (already in the harness's expected format; "" means print nothing).
func (m *Manager) Handle(ctx context.Context, harness, eventName string, payload json.RawMessage) (string, error) {
	start := time.Now()
	ev, err := parse(harness, eventName, payload)
	if err != nil {
		m.record(harness, eventName, "", err, start, "unreadable payload")
		return "", err
	}
	project := core.ResolveProject(ev.Cwd)
	out, summary, err := m.dispatch(ctx, harness, ev, project)
	m.record(harness, ev.Name, project, err, start, summary)
	return out, err
}

// record logs a hook call to the activity feed.
func (m *Manager) record(harness, name, project string, err error, start time.Time, summary string) {
	a := core.Activity{Source: "hook", Harness: harness, Tool: name, Summary: summary, Project: project, DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		a.Error = err.Error()
	}
	m.core.RecordActivity(a)
}

// dispatch performs the memory work for one normalised event and returns the harness output and a summary for the feed.
func (m *Manager) dispatch(ctx context.Context, harness string, ev event, project string) (out, summary string, err error) {
	switch ev.Kind {
	case kindStart:
		text, err := m.core.AnchorFor(ctx, project, ev.Session)
		if err != nil {
			return "", "anchor failed", err
		}
		return render(harness, ev.Name, text), "injected session anchor", nil

	case kindPrompt:
		if ev.Session != "" && len(strings.TrimSpace(ev.Prompt)) >= 3 {
			if err := m.core.CaptureTurn(ctx, harness, ev.Session, project, "user", ev.Prompt); err != nil {
				return "", "capture failed", err
			}
		}
		sf, err := m.core.SurfaceForPrompt(ctx, ev.Prompt, project)
		if err != nil {
			return "", "surface failed", err
		}
		if sf.Text == "" {
			return render(harness, ev.Name, ""), "quiet (nothing relevant)", nil
		}
		return render(harness, ev.Name, sf.Text), fmt.Sprintf("surfaced %d note(s)", len(sf.Items)), nil

	case kindAssistant:
		if ev.Recursive {
			return render(harness, ev.Name, ""), "skipped (re-entrant stop)", nil
		}
		if ev.Session == "" {
			return render(harness, ev.Name, ""), "no session id; not captured", nil
		}
		if err := m.core.CaptureTurn(ctx, harness, ev.Session, project, "assistant", ev.Text); err != nil {
			return "", "capture failed", err
		}
		// Checkpoint every turn: SessionEnd is not guaranteed (a killed
		// terminal never fires it, VS Code has none), so the raw capture is
		// refreshed now and simply reopened by the next turn.
		if err := m.core.CloseSession(ctx, ev.Session, "checkpoint"); err != nil {
			return "", "checkpoint failed", err
		}
		return render(harness, ev.Name, ""), "captured assistant turn", nil

	case kindCompact:
		if ev.Session == "" {
			return "", "no session id", nil
		}
		if err := m.core.CloseSession(ctx, ev.Session, "compact"); err != nil {
			return "", "checkpoint failed", err
		}
		return "", "checkpointed before compaction", nil

	case kindEnd:
		if ev.Session == "" {
			return "", "no session id", nil
		}
		reason := ev.Reason
		if reason == "" {
			reason = "end"
		}
		if err := m.core.CloseSession(ctx, ev.Session, reason); err != nil {
			return "", "close failed", err
		}
		return "", "session closed (" + reason + ")", nil
	}
	return "", "ignored " + ev.Name, nil
}

// render formats injected context for a harness's event. Events that cannot
// inject, and an empty text, produce the harness's neutral reply.
func render(harness, name, text string) string {
	text = truncateRunes(text, maxInject)
	switch harness {
	case Cursor:
		switch name {
		case "sessionStart":
			if text == "" {
				return "{}"
			}
			return mustJSON(map[string]any{"additional_context": text})
		case "beforeSubmitPrompt":
			// Cursor cannot inject per prompt; it must only be told to continue.
			return mustJSON(map[string]any{"continue": true})
		}
		return "{}"
	default: // claude-code, codex, vscode share the hookSpecificOutput contract
		switch name {
		case "SessionStart", "UserPromptSubmit":
			if text == "" {
				return ""
			}
			return mustJSON(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": name, "additionalContext": text}})
		case "Stop", "SubagentStop":
			return "{}" // Codex requires JSON on Stop; Claude Code accepts it
		}
		return ""
	}
}

// mustJSON marshals v, which is always a plain map here.
func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// truncateRunes shortens s to at most n runes, marking the cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// payload is the union of the fields the supported harnesses send.
type payload struct {
	HookEventName        string   `json:"hook_event_name"`
	SessionID            string   `json:"session_id"`
	ConversationID       string   `json:"conversation_id"`
	Cwd                  string   `json:"cwd"`
	WorkspaceRoots       []string `json:"workspace_roots"`
	Prompt               string   `json:"prompt"`
	LastAssistantMessage string   `json:"last_assistant_message"`
	Text                 string   `json:"text"` // Cursor afterAgentResponse
	Reason               string   `json:"reason"`
	StopHookActive       bool     `json:"stop_hook_active"`
}

// parse normalises a harness payload. eventName (from the command line) wins
// over the payload's own hook_event_name when both are present.
func parse(harness, eventName string, raw json.RawMessage) (event, error) {
	var p payload
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return event{}, fmt.Errorf("unreadable %s payload: %w", harness, err)
		}
	}
	name := eventName
	if name == "" {
		name = p.HookEventName
	}
	ev := event{Name: name, Session: p.SessionID, Cwd: p.Cwd, Recursive: p.StopHookActive}
	if harness == Cursor {
		// Cursor keys everything by conversation and reports workspace roots.
		if p.ConversationID != "" {
			ev.Session = p.ConversationID
		}
		if ev.Cwd == "" && len(p.WorkspaceRoots) > 0 {
			ev.Cwd = p.WorkspaceRoots[0]
		}
	}
	if ev.Session == "" {
		ev.Session = p.ConversationID
	}
	switch harness {
	case ClaudeCode, Codex, VSCode:
		switch name {
		case "SessionStart":
			ev.Kind = kindStart
		case "UserPromptSubmit":
			ev.Kind, ev.Prompt = kindPrompt, p.Prompt
		case "Stop":
			ev.Kind, ev.Text = kindAssistant, p.LastAssistantMessage
		case "PreCompact":
			ev.Kind = kindCompact
		case "SessionEnd":
			ev.Kind, ev.Reason = kindEnd, p.Reason
		}
	case Cursor:
		switch name {
		case "sessionStart":
			ev.Kind = kindStart
		case "beforeSubmitPrompt":
			ev.Kind, ev.Prompt = kindPrompt, p.Prompt
		case "afterAgentResponse":
			ev.Kind, ev.Text = kindAssistant, p.Text
		case "preCompact":
			ev.Kind = kindCompact
		case "sessionEnd":
			ev.Kind, ev.Reason = kindEnd, p.Reason
		}
	default:
		return ev, fmt.Errorf("unknown harness %q (want one of %s)", harness, strings.Join(Harnesses, ", "))
	}
	return ev, nil
}

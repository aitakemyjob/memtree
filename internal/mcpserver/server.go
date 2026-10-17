// Package mcpserver exposes memtree to coding agents over the Model Context
// Protocol. Agents recall, save, classify and consolidate memories here; the
// user can watch every call in the UI and switch agent writes off.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/memtree/internal/core"
)

const instructions = `memtree is the user's persistent, local memory for coding agents. It remembers decisions,
lessons, preferences and summaries of past sessions across conversations and harnesses, organised in a
category lattice (slash paths like project/architecture/db or knowledge/tools/redis).

- Pass project = your working directory on every call; memtree maps it to the repo root. Global memories
  (project "") are visible everywhere; project memories only in their project.
- Recall before guessing: call recall when the user refers to earlier work, past decisions, conventions,
  or "what we did last time". Results carry ids, ages and why they matched; older notes may be stale, so
  check them against the code.
- Save what will matter later with memory_save: decisions (and why), hard-won lessons, user preferences,
  and a short summary at the end of meaningful work. Classify it yourself: give lattice_paths (1-4 paths
  rooted at an existing axis; call lattice_browse to reuse existing categories), kind, importance
  (1 routine, 1.5 decision, 2.5 hard-won lesson), keywords, decisions and open_loops.
- Hooks capture raw sessions automatically. captures_pending lists ones still needing a summary; read one
  with capture_transcript and memory_save a summary with kind "session" and its session_id.
- Keep memory tidy: memory_supersede when something changed, memory_archive when it is wrong,
  lattice_tag to (re)classify, consolidation_candidates + memory_compress to distil old related notes.
- memtree runs no model of its own: all summarising and classifying is your job.`

// Server is the MCP server.
type Server struct {
	core *core.Service
	srv  *mcp.Server
}

// New creates the server with every tool registered and logs each newly
// connected client to the activity feed.
func New(c *core.Service) *Server {
	s := &Server{core: c}
	s.srv = mcp.NewServer(&mcp.Implementation{Name: "memtree", Title: "memtree", Version: c.Version}, &mcp.ServerOptions{
		Instructions: instructions,
		InitializedHandler: func(ctx context.Context, req *mcp.InitializedRequest) {
			name := "agent"
			if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
				name = p.ClientInfo.Name
			}
			c.RecordActivity(core.Activity{Source: "mcp", Harness: name, Tool: "connect", Summary: name + " connected"})
		},
	})
	s.register()
	return s
}

// MCP returns the underlying SDK server (used by tests and stdio mode).
func (s *Server) MCP() *mcp.Server { return s.srv }

// Handler serves the streamable HTTP transport.
func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.srv },
		&mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute})
}

// Run serves MCP over stdio (`memtree mcp --standalone`).
func (s *Server) Run(ctx context.Context) error { return s.srv.Run(ctx, &mcp.StdioTransport{}) }

// ok wraps text in a successful tool result.
func ok(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// addTool registers a text-returning tool and records every call in the
// activity feed so the user can see what the agent is doing with memory.
func addTool[In any](s *Server, t *mcp.Tool, summary func(In) (string, string), h func(context.Context, In) (string, error)) {
	mcp.AddTool(s.srv, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		out, err := h(ctx, in)
		sum, project := summary(in)
		harness := "agent"
		if req != nil && req.Session != nil {
			if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil && p.ClientInfo.Name != "" {
				harness = p.ClientInfo.Name
			}
		}
		a := core.Activity{Source: "mcp", Harness: harness, Tool: t.Name, Summary: sum, Project: project, DurationMS: time.Since(start).Milliseconds()}
		if err != nil {
			a.Error = err.Error()
		}
		s.core.RecordActivity(a)
		if err != nil {
			if errors.Is(err, core.ErrAgentWritesDisabled) {
				return nil, nil, fmt.Errorf("%w; the user can enable them under Settings → Agent access", err)
			}
			return nil, nil, err
		}
		return ok(out), nil, nil
	})
}

// boolp returns a pointer to b (annotation fields are optional booleans).
func boolp(b bool) *bool { return &b }

// readOnly annotates a tool that only reads memory.
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: boolp(false)}
}

// writes annotates a tool that changes memory but never deletes it.
func writes(title string, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: boolp(false), IdempotentHint: idempotent, OpenWorldHint: boolp(false)}
}
